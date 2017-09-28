package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from cloud providers or localized storage
import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type gsStorage struct {
	project string
	bucket  string
	client  *storage.Client
}

func (*gsStorage) getCred(env map[string]string) (opts option.ClientOption, err errors.Error) {
	val, isPresent := os.LookupEnv("GOOGLE_FIREBASE_CREDENTIALS")
	if !isPresent {
		if val, isPresent = env["GOOGLE_FIREBASE_CREDENTIALS"]; !isPresent {

			return nil, errors.New(`the environment variable GOOGLE_FIREBASE_CREDENTIALS was not set,
		fix this by saving your firebase credentials.  To do this use the Firebase Admin SDK 
		panel inside the Project Settings menu and then navigate to Setting -> Service Accounts 
		section.  This panel gives the option of generating private keys for your account.  
		Creating a key will overwrite any existing key. Save the generated key into a safe 
		location and define an environment variable GOOGLE_FIREBASE_CREDENTIALS to point at this file`)
		}
	}
	return option.WithServiceAccountFile(val), nil
}

func NewGSstorage(projectID string, env map[string]string, bucket string, validate bool, timeout time.Duration) (s *gsStorage, err errors.Error) {

	s = &gsStorage{
		project: projectID,
		bucket:  bucket,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cred, err := s.getCred(env)
	if err != nil {
		return nil, err
	}

	client, errGo := storage.NewClient(ctx, cred)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	s.client = client

	if validate {
		// Validate the bucket during the NewBucket to give an early warning of issues
		buckets := s.client.Buckets(ctx, projectID)
		for {
			attrs, errGo := buckets.Next()
			if errGo == iterator.Done {
				return nil, errors.New("bucket not found").With("stack", stack.Trace().TrimRuntime()).With("project", projectID).With("bucket", bucket)
			}
			if errGo != nil {
				return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			if attrs.Name == bucket {
				break
			}
		}
	}

	return s, nil
}

func (s *gsStorage) Close() {
	s.client.Close()
}

// Hash returns an MD5 of the contents of the file that can be used by caching and other functions
// to track storage changes etc
//
func (s *gsStorage) Hash(name string, timeout time.Duration) (hash string, err errors.Error) {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	attrs, errGo := s.client.Bucket(s.bucket).Object(name).Attrs(ctx)
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return hex.EncodeToString(attrs.MD5), nil
}

// Detect is used to extract from content on the storage server what the type of the payload is
// that is present on the server
//
func (s *gsStorage) Detect(name string, timeout time.Duration) (fileType string, err errors.Error) {
	switch filepath.Ext(name) {
	case "gzip", "gz":
		return "application/x-gzip", nil
	case "zip":
		return "application/zip", nil
	case "tgz": // Non standard extension as a result of stuioml python code
		return "application/bzip2", nil
	case "tb2", "tbz", "tbz2", "bzip2", "bz2": // Standard bzip2 extensions
		return "application/bzip2", nil
	default:
		return "application/bzip2", nil
	}
}

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *gsStorage) Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (err errors.Error) {

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		errGo := fmt.Errorf("%s is not a directory", output)
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	fileType, err := s.Detect(name, timeout)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, errGo := s.client.Bucket(s.bucket).Object(name).NewReader(ctx)
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
		case "application/bzip2":
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
				inReader, errGo = gzip.NewReader(io.TeeReader(obj, tap))
				inReader = ioutil.NopCloser(io.TeeReader(obj, tap))
			} else {
				inReader = ioutil.NopCloser(obj)
			}
		}
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", output)
		}
		defer inReader.Close()

		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errGo == io.EOF {
				break
			} else if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			path := filepath.Join(output, header.Name)
			info := header.FileInfo()
			if info.IsDir() {
				if errGo = os.MkdirAll(path, info.Mode()); errGo != nil {
					return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			_, errGo = io.Copy(file, tarReader)
			file.Close()
			if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	} else {
		f, errGo := os.Create(filepath.Join(output, name))
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		if _, errGo = io.Copy(outf, obj); errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		outf.Flush()
	}
	return nil
}

// Deposit directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *gsStorage) Deposit(src string, dest string, timeout time.Duration) (err errors.Error) {

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj := s.client.Bucket(s.bucket).Object(dest).NewWriter(ctx)
	defer obj.Close()

	var outw io.Writer

	if strings.HasSuffix(dest, ".tgz") ||
		strings.HasSuffix(dest, ".tar.gz") ||
		strings.HasSuffix(dest, ".tar.gzip") {
		outZ := gzip.NewWriter(obj)
		defer outZ.Close()
		outw = outZ
	} else {
		outw = bufio.NewWriter(obj)
	}

	tw := tar.NewWriter(outw)
	defer tw.Close()

	return filepath.Walk(src, func(file string, fi os.FileInfo, err error) (errGo error) {

		// return on any error
		if err != nil {
			return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		}

		link := ""
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			if link, errGo = os.Readlink(file); errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}

		// create a new dir/file header
		header, errGo := tar.FileInfoHeader(fi, link)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, src, "", -1), string(filepath.Separator))

		// write the header
		if errGo = tw.WriteHeader(header); errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		// return on directories since there will be no content to tar, only headers
		if !fi.Mode().IsRegular() {
			return nil
		}

		// open files for taring
		f, errGo := os.Open(file)
		defer f.Close()
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		// copy file data into tar writer
		if _, errGo := io.Copy(tw, f); errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		return nil
	}).(errors.Error)
}
