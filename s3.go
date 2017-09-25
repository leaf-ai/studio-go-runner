package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/minio/minio-go"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
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
func NewS3storage(projectID string, env map[string]string, endpoint string, bucket string, validate bool, timeout time.Duration) (s *s3Storage, err errors.Error) {

	s = &s3Storage{
		project: projectID,
		bucket:  bucket,
	}

	access := env["AWS_ACCESS_KEY_ID"]
	if len(access) == 0 {
		access = env["AWS_ACCESS_KEY"]
	}
	secret := env["AWS_SECRET_ACCESS_KEY"]
	if len(secret) == 0 {
		secret = env["AWS_SECRET_KEY"]
	}

	errGo := fmt.Errorf("")

	// When using official S3 then the region will be encoded into the endpoint and in order to
	// prevent cross region authentication problems we will need to extract it and use the minio
	// NewWithRegion function.
	//
	// For additional information about regions and naming for S3 endpoints please review the following,
	// http://docs.aws.amazon.com/general/latest/gr/rande.html#s3_region
	//
	hostParts := strings.SplitN(endpoint, ":", 2)
	if strings.HasPrefix(hostParts[0], "s3-") && strings.HasSuffix(hostParts[0], ".amazonaws.com") {
		region := strings.TrimPrefix(hostParts[0], "s3-")
		region = strings.TrimSuffix(region, ".amazonaws.com")
		s.client, errGo = minio.NewWithRegion(endpoint, access, secret, true, region)
	} else {
		// Initialize minio client object.
		s.client, errGo = minio.New(endpoint, access, secret, true)
	}
	if errGo != nil {
		return s, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", output)
	}
	if !info.IsDir() {
		return errors.New("a directory was not used, or did not exist").With("stack", stack.Trace().TrimRuntime()).With("dir", output)
	}

	obj, errGo := s.client.GetObject(s.bucket, name)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", output)
	}
	defer obj.Close()

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {
		var inReader io.ReadCloser
		if strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".tar.gzip") {
			if tap != nil {
				// Create a stack of reader that first tee off any data read to a tap
				// the tap being able to send data to things like caches etc
				//
				// Second in the stack of readers after the TAP is a decompression reader
				inReader, errGo = gzip.NewReader(io.TeeReader(obj, tap))
			} else {
				inReader, errGo = gzip.NewReader(obj)
			}
			if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", output)
			}
		} else {
			inReader = ioutil.NopCloser(bufio.NewReader(obj))
		}
		defer inReader.Close()

		// Last in the stack is a tar file handling reader
		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errGo == io.EOF {
				break
			} else if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", output)
			}

			path := filepath.Join(output, header.Name)

			if len(header.Linkname) != 0 {
				if errGo = os.Symlink(header.Linkname, path); errGo != nil {
					return errors.Wrap(errGo, "symbolic link create failed").With("stack", stack.Trace().TrimRuntime()).With("file", output)
				}
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if info.IsDir() {
					if errGo = os.MkdirAll(path, os.FileMode(header.Mode)); errGo != nil {
						return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
					}
				}
			case tar.TypeReg, tar.TypeRegA:

				file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
				if errGo != nil {
					return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
				}

				_, errGo = io.Copy(file, tarReader)
				file.Close()
				if errGo != nil {
					return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
				}
			default:
				errGo = fmt.Errorf("unknown tar archive type '%c'", header.Typeflag)
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
			}
		}
	} else {
		path := filepath.Join(output, name)
		f, errGo := os.Create(path)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
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
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", path)
		}
		outf.Flush()
	}
	return nil
}

// Return directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *s3Storage) Deposit(src string, dest string, timeout time.Duration) (err errors.Error) {

	if !strings.HasSuffix(dest, ".tgz") &&
		!strings.HasSuffix(dest, ".tar.gz") &&
		!strings.HasSuffix(dest, ".tar.gzip") {
		return errors.New("uploads must be compressed tar files").With("stack", stack.Trace().TrimRuntime()).With("file", dest)
	}

	pr, pw := io.Pipe()

	outw := gzip.NewWriter(pw)

	tw := tar.NewWriter(outw)

	go func() {
		defer pw.Close()
		defer outw.Close()
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
