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
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	bzip2w "github.com/dsnet/compress/bzip2"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type gsStorage struct {
	project string
	bucket  string
	client  *storage.Client
}

func NewGSstorage(projectID string, creds string, env map[string]string, bucket string, validate bool, timeout time.Duration) (s *gsStorage, err errors.Error) {

	s = &gsStorage{
		project: projectID,
		bucket:  bucket,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, errGo := storage.NewClient(ctx, option.WithCredentialsFile(creds))
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

// Fetch is used to retrieve a file from a well known google storage bucket and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *gsStorage) Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (warns []errors.Error, err errors.Error) {

	errors := errors.With("output", output).With("name", name)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		errGo = fmt.Errorf("%s is not a directory", output)
		return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	fileType, w := MimeFromExt(name)
	if w != nil {
		warns = append(warns, w)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj, errGo := s.client.Bucket(s.bucket).Object(name).NewReader(ctx)
	if errGo != nil {
		return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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
			return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer inReader.Close()

		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errGo == io.EOF {
				break
			} else if errGo != nil {
				return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			path := filepath.Join(output, header.Name)
			info := header.FileInfo()
			if info.IsDir() {
				if errGo = os.MkdirAll(path, info.Mode()); errGo != nil {
					return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if errGo != nil {
				return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			_, errGo = io.Copy(file, tarReader)
			file.Close()
			if errGo != nil {
				return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("timeout", timeout.String())
			}
		}
	} else {
		errGo := os.MkdirAll(output, 0700)
		if errGo != nil {
			return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("output", output)
		}
		path := filepath.Join(output, filepath.Base(name))
		f, errGo := os.Create(path)
		if errGo != nil {
			return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		if _, errGo = io.Copy(outf, obj); errGo != nil {
			return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		outf.Flush()
	}
	return warns, nil
}

// Deposit directories as compressed artifacts to the firebase storage for an
// experiment
//
func (s *gsStorage) Deposit(src string, dest string, timeout time.Duration) (warns []errors.Error, err errors.Error) {

	if !IsTar(dest) {
		return warns, errors.New("uploads must be tar, or tar compressed files").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	obj := s.client.Bucket(s.bucket).Object(dest).NewWriter(ctx)
	defer obj.Close()

	files, err := NewTarWriter(src)
	if err != nil {
		return warns, err
	}

	if !files.HasFiles() {
		return warns, nil
	}

	var outw io.Writer

	typ, w := MimeFromExt(dest)
	warns = append(warns, w)

	switch typ {
	case "application/tar", "application/octet-stream":
		outw = bufio.NewWriter(obj)
	case "application/bzip2":
		outZ, errGo := bzip2w.NewWriter(obj, &bzip2w.WriterConfig{Level: 6})
		if err != nil {
			return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer outZ.Close()
		outw = outZ
	case "application/x-gzip":
		outZ := gzip.NewWriter(obj)
		defer outZ.Close()
		outw = outZ
	case "application/zip":
		return warns, errors.New("only tar archives are supported").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	default:
		return warns, errors.New("unrecognized upload compression").With("stack", stack.Trace().TrimRuntime()).With("key", dest)
	}

	tw := tar.NewWriter(outw)
	defer tw.Close()

	if err = files.Write(tw); err != nil {
		return warns, err.(errors.Error)
	}
	return warns, nil
}
