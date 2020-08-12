// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

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

	humanize "github.com/dustin/go-humanize"
	"github.com/minio/minio-go/pkg/credentials"

	"github.com/minio/minio-go"

	bzip2w "github.com/dsnet/compress/bzip2"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	s3CA   = flag.String("s3-ca", "", "Used to specify a PEM file for the CA used in securing the S3/Minio connection")
	s3Cert = flag.String("s3-cert", "", "Used to specify a cert file for securing the S3/Minio connection, do not use with the s3-pem option")
	s3Key  = flag.String("s3-key", "", "Used to specify a key file for securing the S3/Minio connection, do not use with the s3-pem option")
)

// StorageImpl is a type that describes the implementation of an S3 storage entity
type StorageImpl int

const (
	// MinioImpl is a minio implementation of an S3 resource
	MinioImpl StorageImpl = iota
	// S3Impl is the references aws implementation of an S3 resource
	S3Impl
)

type s3Storage struct {
	storage    StorageImpl
	endpoint   string
	project    string
	bucket     string
	key        string
	client     *minio.Client
	anonClient *minio.Client
}

// NewS3storage is used to initialize a client that will communicate with S3 compatible storage.
//
// S3 configuration will only be respected using the AWS environment variables.
//
func NewS3storage(ctx context.Context, projectID string, creds string, env map[string]string, endpoint string,
	bucket string, key string, validate bool, useSSL bool) (s *s3Storage, err kv.Error) {

	s = &s3Storage{
		storage:  S3Impl,
		endpoint: endpoint,
		project:  projectID,
		bucket:   bucket,
		key:      key,
	}

	access := ""
	secret := ""
	for k, v := range env {
		switch strings.ToUpper(k) {
		case "AWS_ACCESS_KEY_ID", "MINIO_ACCESS_KEY":
			access = v
		case "AWS_SECRET_ACCESS_KEY", "MINIO_SECRET_KEY":
			secret = v
		case "MINIO_TEST_SERVER":
			s.storage = MinioImpl
			if len(s.endpoint) == 0 {
				s.endpoint = v
			}
		}
	}

	// When using official S3 then the region will be encoded into the endpoint and in order to
	// prevent cross region authentication problems we will need to extract it and use the minio
	// NewWithOptions function and specify the region explicitly to reduce lookups, minio does
	// the processing to get a well known DNS name in these cases.
	//
	// For additional information about regions and naming for S3 endpoints please review the following,
	// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
	//
	region := ""
	if s.storage == S3Impl {
		region = env["AWS_DEFAULT_REGION"]

		if s.endpoint != "s3.amazonaws.com" {
			if (strings.HasPrefix(s.endpoint, "s3-") || strings.HasPrefix(s.endpoint, "s3.")) &&
				strings.HasSuffix(s.endpoint, ".amazonaws.com") {
				region = s.endpoint[3:]
				region = strings.TrimSuffix(region, ".amazonaws.com")
				// Revert to a single well known address for DNS lookups to improve interoperability
				// when running in k8s etc
				s.endpoint = "s3.amazonaws.com"
				useSSL = true
			}
		}

		if len(region) == 0 {
			msg := "the AWS region is missing from the studioML request, and could not be deduced from the endpoint"
			return nil, kv.NewError(msg).With("endpoint", s.endpoint).With("stack", stack.Trace().TrimRuntime())
		}
	}

	// The use of SSL is mandated at this point to ensure that data protections
	// are effective when used by callers
	//
	pemData := []byte{}
	cert := tls.Certificate{}
	errGo := fmt.Errorf("")
	_ = errGo // Bypass the ineffectual assignment check

	if len(*s3Cert) != 0 || len(*s3Key) != 0 {
		if len(*s3Cert) == 0 || len(*s3Key) == 0 {
			return nil, kv.NewError("the s3-cert and s3-key files when used must both be specified")
		}
		if cert, errGo = tls.LoadX509KeyPair(*s3Cert, *s3Key); errGo != nil {
			return nil, kv.Wrap(errGo)
		}
		useSSL = true
	}

	if len(*s3CA) != 0 {
		stat, errGo := os.Stat(*s3CA)
		if errGo != nil {
			return nil, kv.Wrap(errGo, "unable to read a PEM, or Certificate file from disk for S3 security")
		}
		if stat.Size() > 128*1024 {
			return nil, kv.NewError("the PEM, or Certificate file is suspicously large, too large to be a PEM file")
		}
		if pemData, errGo = ioutil.ReadFile(*s3CA); errGo != nil {
			return nil, kv.Wrap(errGo, "PEM, or Certificate file read failed").With("stack", stack.Trace().TrimRuntime())

		}
		if len(pemData) == 0 {
			return nil, kv.NewError("PEM, or Certificate file was empty, PEM data is needed when the file name is specified")
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
	if s.client, errGo = minio.NewWithOptions(s.endpoint, &options); errGo != nil {
		return nil, kv.Wrap(errGo).With("endpoint", s.endpoint, "options", fmt.Sprintf("%+v", options)).With("stack", stack.Trace().TrimRuntime())
	}

	anonOptions := minio.Options{
		// Using empty values seems to be the most appropriate way of getting anonymous access
		// however none of this is documented any where I could find.  This is the only way
		// I could get it to work without panics from the libraries being used.
		Creds:        credentials.NewStaticV4("", "", ""),
		Secure:       useSSL,
		Region:       region,
		BucketLookup: minio.BucketLookupPath,
	}
	if s.anonClient, errGo = minio.NewWithOptions(s.endpoint, &anonOptions); errGo != nil {
		return nil, kv.Wrap(errGo).With("endpoint", s.endpoint, "options", fmt.Sprintf("%+v", options)).With("stack", stack.Trace().TrimRuntime())
	}

	if useSSL {
		caCerts := &x509.CertPool{}

		if len(*s3CA) != 0 {
			if !caCerts.AppendCertsFromPEM(pemData) {
				return nil, kv.NewError("PEM Data could not be added to the system default certificate pool").With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			// First load the default CA's
			caCerts, errGo = x509.SystemCertPool()
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}

		s.client.SetCustomTransport(&http.Transport{
			TLSClientConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
				RootCAs:      caCerts,
			},
		})
		s.anonClient.SetCustomTransport(&http.Transport{
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
func (s *s3Storage) Hash(ctx context.Context, name string) (hash string, err kv.Error) {
	key := name
	if len(key) == 0 {
		key = s.key
	}
	info, errGo := s.client.StatObject(s.bucket, key, minio.StatObjectOptions{})
	if errGo != nil {
		if minio.ToErrorResponse(errGo).Code == "AccessDenied" {
			// Try accessing the artifact without any credentials
			info, errGo = s.anonClient.StatObject(s.bucket, key, minio.StatObjectOptions{})
		}
	}
	if errGo != nil {
		return "", kv.Wrap(errGo).With("bucket", s.bucket).With("key", key).With("stack", stack.Trace().TrimRuntime())
	}
	return info.ETag, nil
}

func (s *s3Storage) listObjects(keyPrefix string) (names []string, warnings []kv.Error, err kv.Error) {
	names = []string{}
	isRecursive := true

	// Create a done channel to control 'ListObjects' go routine.
	doneCh := make(chan struct{})

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	// Try all available clients with possibly various credentials to get things
	for _, aClient := range []*minio.Client{s.client, s.anonClient} {
		objectCh := aClient.ListObjects(s.bucket, keyPrefix, isRecursive, doneCh)
		for object := range objectCh {
			if object.Err != nil {
				if minio.ToErrorResponse(object.Err).Code == "AccessDenied" {
					continue
				}
				return nil, nil, kv.Wrap(object.Err).With("bucket", s.bucket, "keyPrefix", keyPrefix).With("stack", stack.Trace().TrimRuntime())
			}
			names = append(names, object.Key)
		}
	}
	return names, nil, err
}

// Gather is used to retrieve files prefixed with a specific key.  It is used to retrieve the individual files
// associated with a previous Hoard operation.
//
func (s *s3Storage) Gather(ctx context.Context, keyPrefix string, outputDir string, tap io.Writer) (warnings []kv.Error, err kv.Error) {
	// Retrieve a list of the known keys that match the key prefix

	names := []string{}
	_ = names // Bypass the ineffectual assignment check

	names, warnings, err = s.listObjects(keyPrefix)

	// Download these files
	for _, key := range names {
		w, e := s.Fetch(ctx, key, false, outputDir, tap)
		if len(w) != 0 {
			warnings = append(warnings, w...)
		}
		if e != nil {
			err = e
		}
	}
	return warnings, err
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *s3Storage) Fetch(ctx context.Context, name string, unpack bool, output string, tap io.Writer) (warns []kv.Error, err kv.Error) {

	key := name
	if len(key) == 0 {
		key = s.key
	}
	errCtx := kv.With("output", output).With("name", name).
		With("bucket", s.bucket).With("key", key).With("endpoint", s.endpoint)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return warns, errCtx.NewError("a directory was not used, or did not exist").With("stack", stack.Trace().TrimRuntime())
	}

	fileType, w := MimeFromExt(name)
	if w != nil {
		warns = append(warns, w)
	}

	obj, errGo := s.client.GetObjectWithContext(ctx, s.bucket, key, minio.GetObjectOptions{})
	if errGo == nil {
		// Errors can be delayed until the first interaction with the storage platform so
		// we exercise access to the meta data at least to validate the object we have
		_, errGo = obj.Stat()
	}
	if errGo != nil {
		if minio.ToErrorResponse(errGo).Code == "AccessDenied" {
			obj, errGo = s.anonClient.GetObjectWithContext(ctx, s.bucket, key, minio.GetObjectOptions{})
			if errGo == nil {
				// Errors can be delayed until the first interaction with the storage platform so
				// we exercise access to the meta data at least to validate the object we have
				_, errGo = obj.Stat()
			}
		}
		if errGo != nil {
			return warns, errCtx.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
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

			path, _ := filepath.Abs(filepath.Join(output, header.Name))
			if !strings.HasPrefix(path, output) {
				fmt.Println(errCtx.NewError("archive file name escaped").With("path", path, "output", output, "filename", header.Name).With("stack", stack.Trace().TrimRuntime()).Error())
				return warns, errCtx.NewError("archive file name escaped").With("filename", header.Name).With("stack", stack.Trace().TrimRuntime())
			}

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

// uploadFile can be used to transmit a file to the S3 server using a fully qualified file
// name and key
//
func (s *s3Storage) uploadFile(ctx context.Context, src string, dest string) (err kv.Error) {
	if ctx.Err() != nil {
		return kv.NewError("upload context cancelled").With("stack", stack.Trace().TrimRuntime()).With("src", src, "bucket", s.bucket, "key", dest)
	}

	file, errGo := os.Open(filepath.Clean(src))
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("src", src)
	}
	defer file.Close()

	fileStat, errGo := file.Stat()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("src", src)
	}

	xfered, errGo := s.client.PutObjectWithContext(ctx, s.bucket, dest, file, fileStat.Size(), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("src", src, "bucket", s.bucket, "key", dest)
	}
	if xfered != fileStat.Size() {
		shortage := uint64(fileStat.Size() - xfered)

		err := kv.NewError("upload truncated").With("stack", stack.Trace().TrimRuntime())
		return err.With("shortage", humanize.Bytes(shortage), "src", src, "bucket", s.bucket, "key", dest)
	}
	return nil
}

// Hoard is used to upload the contents of a directory to the storage server as individual files rather than a single
// archive
//
func (s *s3Storage) Hoard(ctx context.Context, srcDir string, keyPrefix string) (warnings []kv.Error, err kv.Error) {

	prefix := keyPrefix
	if len(prefix) == 0 {
		prefix = s.key
	}

	// Walk files taking each uploadable file and placing into a collection
	files := []string{}
	errGo := filepath.Walk(srcDir, func(file string, fi os.FileInfo, err error) error {
		if fi.IsDir() {
			return nil
		}
		// We have a file include it in the upload list
		files = append(files, file)

		return nil
	})
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Upload files
	for _, aFile := range files {
		key := filepath.Join(prefix, strings.TrimPrefix(aFile, srcDir))
		if err = s.uploadFile(ctx, aFile, key); err != nil {
			warnings = append(warnings, err)
		}
	}

	if len(warnings) != 0 {
		err = kv.NewError("one or more uploads failed").With("stack", stack.Trace().TrimRuntime()).With("src", srcDir, "warnings", warnings)
	}

	return warnings, err
}

// Return directories as compressed artifacts to the AWS storage for an
// experiment
//
func (s *s3Storage) Deposit(ctx context.Context, src string, dest string) (warns []kv.Error, err kv.Error) {

	if !IsTar(dest) {
		return warns, kv.NewError("uploads must be tar, or tar compressed files").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
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

	swErrorC := make(chan kv.Error)
	go streamingWriter(pr, pw, files, dest, swErrorC)

	s3ErrorC := make(chan kv.Error)
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

func (s *s3Storage) s3Put(key string, pr *io.PipeReader, errorC chan kv.Error) {

	errS := kv.With("key", key).With("bucket", s.bucket)

	defer func() {
		if r := recover(); r != nil {
			errorC <- errS.NewError(fmt.Sprint(r)).With("stack", stack.Trace().TrimRuntime())
		}
		close(errorC)
	}()
	if _, errGo := s.client.PutObject(s.bucket, key, pr, -1, minio.PutObjectOptions{}); errGo != nil {
		errorC <- errS.Wrap(minio.ToErrorResponse(errGo)).With("stack", stack.Trace().TrimRuntime())
		return
	}
}

type errSender struct {
	errorC chan kv.Error
}

func (es *errSender) send(err kv.Error) {
	if err != nil {
		select {
		case es.errorC <- err:
		case <-time.After(30 * time.Millisecond):
		}
	}
}

func streamingWriter(pr *io.PipeReader, pw *io.PipeWriter, files *TarWriter, dest string, errorC chan kv.Error) {

	sender := errSender{errorC: errorC}

	defer func() {
		if r := recover(); r != nil {
			sender.send(kv.NewError(fmt.Sprint(r)).With("stack", stack.Trace().TrimRuntime()))
		}

		pw.Close()
		close(errorC)
	}()

	typ, w := MimeFromExt(dest)
	sender.send(w)

	switch typ {
	case "application/tar", "application/octet-stream":
		tw := tar.NewWriter(pw)
		if errGo := files.Write(tw); errGo != nil {
			sender.send(errGo)
		}
		tw.Close()
	case "application/bzip2":
		outZ, _ := bzip2w.NewWriter(pw, &bzip2w.WriterConfig{Level: 6})
		tw := tar.NewWriter(outZ)
		if errGo := files.Write(tw); errGo != nil {
			sender.send(errGo)
		}
		tw.Close()
		outZ.Close()
	case "application/x-gzip":
		outZ := gzip.NewWriter(pw)
		tw := tar.NewWriter(outZ)
		if errGo := files.Write(tw); errGo != nil {
			sender.send(errGo)
		}
		tw.Close()
		outZ.Close()
	case "application/zip":
		sender.send(kv.NewError("only tar archives are supported").With("stack", stack.Trace().TrimRuntime()).With("key", dest))
		return
	default:
		sender.send(kv.NewError("unrecognized upload compression").With("stack", stack.Trace().TrimRuntime()).With("key", dest))
		return
	}
}
