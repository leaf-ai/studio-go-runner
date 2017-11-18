package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	s3CA   = flag.String("s3-ca", "", "Used to specify a PEM file for the CA used in securing the S3/Minio connection")
	s3Cert = flag.String("s3-cert", "", "Used to specify a cert file for securing the S3/Minio connection, do not use with the s3-pem option")
	s3Key  = flag.String("s3-key", "", "Used to specify a key file for securing the S3/Minio connection, do not use with the s3-pem option")
)

type s3Storage struct {
	project string
	bucket  string
	client  *minio.Client
}

// NewS3Storage is used to initialize a client that will communicate with S3 compatible storage.
//
// S3 configuration will only be respected using the AWS environment variables.
//
func NewS3storage(projectID string, creds string, env map[string]string, endpoint string, bucket string, validate bool, timeout time.Duration) (s *s3Storage, err errors.Error) {

	s = &s3Storage{
		project: projectID,
		bucket:  bucket,
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

	region := env["AWS_DEFAULT_REGION"]
	if 0 == len(region) {
		return nil, errors.New("the AWS region is missing from the studioML request").With("stack", stack.Trace().TrimRuntime())
	}

	errGo := fmt.Errorf("")

	// The use of SSL is mandated at this point to ensure that data protections
	// are effective when used by callers
	//
	pemData := []byte{}
	cert := tls.Certificate{}
	useSSL := false

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

	// When using official S3 then the region will be encoded into the endpoint and in order to
	// prevent cross region authentication problems we will need to extract it and use the minio
	// NewWithRegion function.
	//
	// For additional information about regions and naming for S3 endpoints please review the following,
	// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
	//
	hostParts := strings.SplitN(endpoint, ":", 2)
	if strings.HasPrefix(hostParts[0], "s3-") && strings.HasSuffix(hostParts[0], ".amazonaws.com") {
		region = strings.TrimPrefix(hostParts[0], "s3-")
		region = strings.TrimSuffix(region, ".amazonaws.com")
	}

	if useSSL {
		url, errGo := url.Parse(endpoint)
		if errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		if len(url.Host) == 0 {
			return nil, errors.New("S3/minio endpoint lacks a scheme, or the host name was not specified").With("stack", stack.Trace().TrimRuntime())
		}
		if url.Scheme != "https" {
			return nil, errors.New("S3/minio endpoint was not specified using https").With("stack", stack.Trace().TrimRuntime())
		}
	}

	if len(region) != 0 {
		if s.client, errGo = minio.NewWithRegion(endpoint, access, secret, useSSL, region); errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	} else {
		// Initialize minio client object.
		if s.client, errGo = minio.New(endpoint, access, secret, useSSL); errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
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
	info, errGo := s.client.StatObject(s.bucket, name)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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
func (s *s3Storage) Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (err errors.Error) {

	errors := errors.With("output", output).With("name", name)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return errors.New("a directory was not used, or did not exist").With("stack", stack.Trace().TrimRuntime())
	}

	fileType := MimeFromExt(name)

	obj, errGo := s.client.GetObject(s.bucket, name)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer inReader.Close()

		// Last in the stack is a tar file handling reader
		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errGo == io.EOF {
				break
			} else if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("fileType", fileType)
			}

			path := filepath.Join(output, header.Name)

			if len(header.Linkname) != 0 {
				if errGo = os.Symlink(header.Linkname, path); errGo != nil {
					return errors.Wrap(errGo, "symbolic link create failed").With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if info.IsDir() {
					if errGo = os.MkdirAll(path, os.FileMode(header.Mode)); errGo != nil {
						return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
					}
				}
			case tar.TypeReg, tar.TypeRegA:

				file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
				if errGo != nil {
					return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}

				_, errGo = io.Copy(file, tarReader)
				file.Close()
				if errGo != nil {
					return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
				}
			default:
				errGo = fmt.Errorf("unknown tar archive type '%c'", header.Typeflag)
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
			}
		}
	} else {
		errGo := os.MkdirAll(output, 0700)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("output", output)
		}
		path := filepath.Join(output, filepath.Base(name))
		f, errGo := os.Create(path)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
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
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", path)
		}
		outf.Flush()
	}
	return nil
}

// Return directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *s3Storage) Deposit(src string, dest string, timeout time.Duration) (err errors.Error) {

	compress := !strings.HasSuffix(dest, ".tar")
	if !strings.HasSuffix(dest, ".tar") &&
		!strings.HasSuffix(dest, ".tgz") &&
		!strings.HasSuffix(dest, ".tar.gz") &&
		!strings.HasSuffix(dest, ".tar.bzip2") &&
		!strings.HasSuffix(dest, ".tar.bz2") &&
		!strings.HasSuffix(dest, ".tar.gzip") {
		return errors.New("uploads must be tar (compressed) files").With("stack", stack.Trace().TrimRuntime()).With("file", dest)
	}

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()

		tw := &tar.Writer{}

		if compress {
			outw := gzip.NewWriter(pw)
			defer outw.Close()

			tw = tar.NewWriter(outw)
		} else {
			tw = tar.NewWriter(pw)
		}
		defer tw.Close()

		filepath.Walk(src, func(file string, fi os.FileInfo, err error) error {

			// return on any error
			if err != nil {
				return err
			}

			link := ""
			if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
				if link, err = os.Readlink(file); err != nil {
					return err
				}
			}

			// create a new dir/file header
			header, err := tar.FileInfoHeader(fi, link)
			if err != nil {
				return err
			}

			// update the name to correctly reflect the desired destination when untaring
			header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

			// write the header
			if err = tw.WriteHeader(header); err != nil {
				return err
			}

			// return on directories since there will be no content to tar, only headers
			if !fi.Mode().IsRegular() {
				return nil
			}

			// open files for taring
			f, err := os.Open(file)
			defer f.Close()
			if err != nil {
				return err
			}

			// copy file data into tar writer
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}

			return nil
		})
	}()

	_, errGo := s.client.PutObjectStreaming(s.bucket, dest, pr)

	pr.Close()

	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", dest)
	}
	return nil
}
