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
func NewS3storage(projectID string, env map[string]string, endpoint string, bucket string, validate bool, timeout time.Duration) (s *s3Storage, err error) {

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
		s.client, err = minio.NewWithRegion(endpoint, access, secret, true, region)
	} else {
		// Initialize minio client object.
		s.client, err = minio.New(endpoint, access, secret, true)
	}
	return s, err
}

func (s *s3Storage) Close() {
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
func (s *s3Storage) Fetch(name string, unpack bool, output string, timeout time.Duration) (err error) {

	// Make sure output is an existing directory
	info, err := os.Stat(output)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", output)
	}

	obj, err := s.client.GetObject(s.bucket, name)
	if err != nil {
		return err
	}
	defer obj.Close()

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {

		var inReader io.ReadCloser
		if strings.HasSuffix(name, ".tgz") ||
			strings.HasSuffix(name, ".tar.gz") ||
			strings.HasSuffix(name, ".tar.gzip") {
			if inReader, err = gzip.NewReader(obj); err != nil {
				return err
			}
		} else {
			inReader = ioutil.NopCloser(bufio.NewReader(obj))
		}
		defer inReader.Close()

		tarReader := tar.NewReader(inReader)

		for {
			header, err := tarReader.Next()
			if err == io.EOF {
				break
			} else if err != nil {
				return err
			}

			path := filepath.Join(output, header.Name)

			if len(header.Linkname) != 0 {
				if err = os.Symlink(header.Linkname, path); err != nil {
					return fmt.Errorf("%s: making symbolic link for: %v", path, err)
				}
				continue
			}

			switch header.Typeflag {
			case tar.TypeDir:
				if info.IsDir() {
					if err = os.MkdirAll(path, os.FileMode(header.Mode)); err != nil {
						return err
					}
				}
			case tar.TypeReg, tar.TypeRegA:

				file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
				if err != nil {
					return err
				}

				_, err = io.Copy(file, tarReader)
				file.Close()
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("unknown tar archive type '%c'", header.Typeflag)
			}
		}
	} else {
		f, err := os.Create(filepath.Join(output, name))
		if err != nil {
			return err
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		_, err = io.Copy(outf, obj)
		if err != nil {
			return err
		}
		outf.Flush()
	}
	return nil
}

// Return directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *s3Storage) Deposit(src string, dest string, timeout time.Duration) (err error) {

	if !strings.HasSuffix(dest, ".tgz") &&
		!strings.HasSuffix(dest, ".tar.gz") &&
		!strings.HasSuffix(dest, ".tar.gzip") {
		return fmt.Errorf("file uploaded to storage must be compressed tar archives, %s was used", dest)
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

	_, err = s.client.PutObjectStreaming(s.bucket, dest, pr)

	pr.Close()

	return err
}
