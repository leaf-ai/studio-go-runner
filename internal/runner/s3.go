package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go/pkg/credentials"

	"github.com/minio/minio-go"

	bzip2w "github.com/dsnet/compress/bzip2"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	s3CA   = flag.String("s3-ca", "", "Used to specify a PEM file for the CA used in securing the S3/Minio connection")
	s3Cert = flag.String("s3-cert", "", "Used to specify a cert file for securing the S3/Minio connection, do not use with the s3-pem option")
	s3Key  = flag.String("s3-key", "", "Used to specify a key file for securing the S3/Minio connection, do not use with the s3-pem option")
)

type s3Storage struct {
	endpoint string
	project  string
	bucket   string
	key      string
	client   *minio.Client
}

// NewS3storage is used to initialize a client that will communicate with S3 compatible storage.
//
// S3 configuration will only be respected using the AWS environment variables.
//
func NewS3storage(projectID string, creds string, env map[string]string, endpoint string,
	bucket string, key string, validate bool, timeout time.Duration, useSSL bool) (s *s3Storage, err errors.Error) {

	s = &s3Storage{
		endpoint: endpoint,
		project:  projectID,
		bucket:   bucket,
		key:      key,
	}

	access := env["AWS_ACCESS_KEY_ID"]
	if 0 == len(access) {
		access = env["AWS_ACCESS_KEY"]
	}
	if 0 == len(access) {
		return nil, errors.New("AWS_ACCESS_KEY_ID is missing from the studioML request").With("stack", stack.Trace().TrimRuntime())
	}
	secret := env["AWS_SECRET_ACCESS_KEY"]
	if 0 == len(secret) {
		secret = env["AWS_SECRET_KEY"]
	}
	if 0 == len(secret) {
		return nil, errors.New("AWS_SECRET_ACCESS_KEY is missing from the studioML request").With("stack", stack.Trace().TrimRuntime())
	}

	// When using official S3 then the region will be encoded into the endpoint and in order to
	// prevent cross region authentication problems we will need to extract it and use the minio
	// NewWithOptions function and specify the region explicitly to reduce lookups, minio does
	// the processing to get a well known DNS name in these cases.
	//
	// For additional information about regions and naming for S3 endpoints please review the following,
	// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
	//
	region := env["AWS_DEFAULT_REGION"]

	if endpoint != "s3.amazonaws.com" {
		if (strings.HasPrefix(endpoint, "s3-") || strings.HasPrefix(endpoint, "s3.")) &&
			strings.HasSuffix(endpoint, ".amazonaws.com") {
			region = endpoint[3:]
			region = strings.TrimSuffix(region, ".amazonaws.com")
			// Revert to a single well known address for DNS lookups to improve interoperability
			// when running in k8s etc
			endpoint = "s3.amazonaws.com"
			useSSL = true
		}
	}

	if len(region) == 0 {
		msg := "the AWS region is missing from the studioML request, and could not be deduced from the endpoint"
		return nil, errors.New(msg).With("endpoint", endpoint).With("stack", stack.Trace().TrimRuntime())
	}

	// The use of SSL is mandated at this point to ensure that data protections
	// are effective when used by callers
	//
	pemData := []byte{}
	cert := tls.Certificate{}
	errGo := fmt.Errorf("")

	if len(*s3Cert) != 0 || len(*s3Key) != 0 {
		if len(*s3Cert) == 0 || len(*s3Key) == 0 {
			return nil, errors.New("the s3-cert and s3-key files when used must both be specified")
		}
		cert, errGo = tls.LoadX509KeyPair(*s3Cert, *s3Key)
		if errGo != nil {
			return nil, errors.Wrap(errGo)
		}
		useSSL = true
	}

	if len(*s3CA) != 0 {
		stat, errGo := os.Stat(*s3CA)
		if errGo != nil {
			return nil, errors.Wrap(errGo, "unable to read a PEM, or Certificate file from disk for S3 security")
		}
		if stat.Size() > 128*1024 {
			return nil, errors.New("the PEM, or Certificate file is suspicously large, too large to be a PEM file")
		}
		if pemData, errGo = ioutil.ReadFile(*s3CA); errGo != nil {
			return nil, errors.Wrap(errGo, "PEM, or Certificate file read failed").With("stack", stack.Trace().TrimRuntime())

		}
		if len(pemData) == 0 {
			return nil, errors.New("PEM, or Certificate file was empty, PEM data is needed when the file name is specified")
		}
		useSSL = true
	}

	// Using the BucketLookupPath strategy to avoid using DNS lookups for the buckets first
	options := minio.Options{
		Creds:        credentials.NewStaticV4(access, secret, ""),
		Secure:       useSSL,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	}

	if s.client, errGo = minio.NewWithOptions(endpoint, &options); errGo != nil {
		return nil, errors.Wrap(errGo).With("options", fmt.Sprintf("%+v", options)).With("stack", stack.Trace().TrimRuntime())
	}

	if useSSL {
		caCerts := &x509.CertPool{}

		if len(*s3CA) != 0 {
			if !caCerts.AppendCertsFromPEM(pemData) {
				return nil, errors.New("PEM Data could not be added to the system default certificate pool").With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			// First load the default CA's
			caCerts, errGo = x509.SystemCertPool()
			if errGo != nil {
				return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}

		s.client.SetCustomTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCerts,
			},
		})
	}

	return s, nil
}

func (s *s3Storage) Close() {
}

// Hash returns aplatform specific MD5 of the contents of the file that can be used by caching and other functions
// to track storage changes etc
//
// The hash on AWS S3 is not a plain MD5 but uses multiple hashes from file
// segments to increase the speed of hashing and also to reflect the multipart download
// processing that was used for the file, for a full explanation please see
// https://stackoverflow.com/questions/12186993/what-is-the-algorithm-to-compute-the-amazon-s3-etag-for-a-file-larger-than-5gb
//
//
func (s *s3Storage) Hash(name string, timeout time.Duration) (hash string, err errors.Error) {
	key := name
	if len(key) == 0 {
		key = s.key
	}
	info, errGo := s.client.StatObject(s.bucket, key, minio.StatObjectOptions{})
	if errGo != nil {
		return "", errors.Wrap(errGo).With("bucket", s.bucket).With("key", key).With("stack", stack.Trace().TrimRuntime())
	}
	return info.ETag, nil
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *s3Storage) Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (warns []errors.Error, err errors.Error) {

	key := name
	if len(key) == 0 {
		key = s.key
	}
	errCtx := errors.With("output", output).With("name", name).
		With("bucket", s.bucket).With("key", key).With("endpoint", s.endpoint).With("timeout", timeout)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return warns, errCtx.New("a directory was not used, or did not exist").With("stack", stack.Trace().TrimRuntime())
	}

	fileType, w := MimeFromExt(name)
	if w != nil {
		warns = append(warns, w)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, errGo := s.client.GetObjectWithContext(ctx, s.bucket, key, minio.GetObjectOptions{})
	if errGo != nil {
		return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer obj.Close()

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {

		var inReader io.ReadCloser

		switch fileType {
		case "application/x-gzip", "application/zip":
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader, errGo = gzip.NewReader(io.TeeReader(obj, tap))
			} else {
				inReader, errGo = gzip.NewReader(obj)
			}
		case "application/bzip2", "application/octet-stream":
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader = ioutil.NopCloser(bzip2.NewReader(io.TeeReader(obj, tap)))
			} else {
				inReader = ioutil.NopCloser(bzip2.NewReader(obj))
			}
		default:
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader = ioutil.NopCloser(io.TeeReader(obj, tap))
			} else {
				inReader = ioutil.NopCloser(obj)
			}
		}
		if errGo != nil {
			return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer inReader.Close()

		// Last in the stack is a tar file handling reader
		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errGo == io.EOF {
				break
			} else if errGo != nil {
				return warns, errCtx.Wrap(errGo).With("fileType", fileType).With("stack", stack.Trace().TrimRuntime())
			}

			path := filepath.Join(output, header.Name)

			if len(header.Linkname) != 0 {
				if errGo = os.Symlink(header.Linkname, path); errGo != nil {
					return warns, errCtx.Wrap(errGo, "symbolic link create failed").With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if info.IsDir() {
					if errGo = os.MkdirAll(path, os.FileMode(header.Mode)); errGo != nil {
						return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
					}
				}
			case tar.TypeReg, tar.TypeRegA:

				// If the file name included directories then these should be created
				if parent, err := filepath.Abs(path); err == nil {
					// implicitly
					_ = os.MkdirAll(filepath.Dir(parent), os.ModePerm)
				}

				file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
				if errGo != nil {
					return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}

				_, errGo = io.Copy(file, tarReader)
				file.Close()
				if errGo != nil {
					return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}
			default:
				errGo = fmt.Errorf("unknown tar archive type '%c'", header.Typeflag)
				return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
			}
		}
	} else {
		errGo := os.MkdirAll(output, 0700)
		if errGo != nil {
			return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("output", output)
		}
		path := filepath.Join(output, filepath.Base(key))
		f, errGo := os.Create(path)
		if errGo != nil {
			return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		if tap != nil {
			// Create a stack of reader that first tee off any data read to a tap
			// the tap being able to send data to things like caches etc
			//
			// Second in the stack of readers after the TAP is a decompression reader
			_, errGo = io.Copy(outf, io.TeeReader(obj, tap))
		} else {
			_, errGo = io.Copy(outf, obj)
		}
		if errGo != nil {
			return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
		}
		outf.Flush()
	}
	return warns, nil
}

// Return directories as compressed artifacts to the AWS storage for an
// experiment
//
func (s *s3Storage) Deposit(src string, dest string, timeout time.Duration) (warns []errors.Error, err errors.Error) {

	if !IsTar(dest) {
		return warns, errors.New("uploads must be tar, or tar compressed files").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	}

	key := dest
	if len(key) == 0 {
		key = s.key
	}

	files, err := NewTarWriter(src)
	if err != nil {
		return warns, err
	}

	if !files.HasFiles() {
		return warns, nil
	}

	pr, pw := io.Pipe()

	swErrorC := make(chan errors.Error)
	go streamingWriter(pr, pw, files, dest, swErrorC)

	s3ErrorC := make(chan errors.Error)
	go s.s3Put(key, pr, s3ErrorC)

	finished := 2
	for {
		select {
		case err = <-swErrorC:
			if nil != err {
				return warns, err
			}
			swErrorC = nil
			finished--
		case err = <-s3ErrorC:
			if nil != err {
				return warns, err
			}
			s3ErrorC = nil
			finished--
		}
		if finished == 0 {
			break
		}
	}

	pr.Close()

	return warns, nil
}

func (s *s3Storage) s3Put(key string, pr *io.PipeReader, errorC chan errors.Error) {
	defer func() {
		if r := recover(); r != nil {
			errorC <- errors.New(fmt.Sprint(r)).With("stack", stack.Trace().TrimRuntime()).With("key", key)
		}
		close(errorC)
	}()
	if _, errGo := s.client.PutObject(s.bucket, key, pr, -1, minio.PutObjectOptions{}); errGo != nil {
		errorC <- errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("key", key)
		return
	}
}

func streamingWriter(pr *io.PipeReader, pw *io.PipeWriter, files *TarWriter, dest string, errorC chan errors.Error) {

	defer func() {
		if r := recover(); r != nil {
			select {
			case errorC <- errors.New(fmt.Sprint(r)).With("stack", stack.Trace().TrimRuntime()):
			case <-time.After(20 * time.Millisecond):
			}
		}

		pw.Close()
		close(errorC)
	}()

	err := errors.New("")

	typ, w := MimeFromExt(dest)
	if w != nil {
		select {
		case errorC <- w:
		case <-time.After(20 * time.Millisecond):
		}
	}
	switch typ {
	case "application/tar", "application/octet-stream":
		tw := tar.NewWriter(pw)
		err = files.Write(tw)
		tw.Close()
	case "application/bzip2":
		outZ, _ := bzip2w.NewWriter(pw, &bzip2w.WriterConfig{Level: 6})
		tw := tar.NewWriter(outZ)
		err = files.Write(tw)
		tw.Close()
		outZ.Close()
	case "application/x-gzip":
		outZ := gzip.NewWriter(pw)
		tw := tar.NewWriter(outZ)
		err = files.Write(tw)
		tw.Close()
		outZ.Close()
	case "application/zip":
		err = errors.New("only tar archives are supported").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
		return
	default:
		err = errors.New("unrecognized upload compression").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
		return
	}
	if err != nil {
		errorC <- err
	}
}
