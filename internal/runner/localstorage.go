package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from local storage

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type localStorage struct {
}

// NewLocalStorage is used to allocate and initialize a struct that acts as a receiver
//
func NewLocalStorage() (s *localStorage, err errors.Error) {
	return &localStorage{}, nil
}

// Close is a NoP unless overridden
func (s *localStorage) Close() {
}

// Hash returns a platform specific hash of the contents of the file that can be used by caching and other functions
// to track storage changes etc
//
func (s *localStorage) Hash(name string, timeout time.Duration) (hash string, err errors.Error) {
	return filepath.Base(name), nil
}

// Fetch is used to retrieve a file from a well known disk directory and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *localStorage) Fetch(name string, unpack bool, output string, tap io.Writer, timeout time.Duration) (warns []errors.Error, err errors.Error) {

	errors := errors.With("output", output).With("name", name)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return warns, errors.New(output+" is not a directory").With("stack", stack.Trace().TrimRuntime())
	}

	fileType, err := MimeFromExt(name)
	if err != nil {
		warns = append(warns, errors.Wrap(err).With("fn", name).With("type", fileType).With("stack", stack.Trace().TrimRuntime()))
	} else {
		warns = append(warns, errors.New("debug").With("fn", name).With("type", fileType).With("stack", stack.Trace().TrimRuntime()))
	}

	obj, errGo := os.Open(name)
	if errGo != nil {
		return warns, errors.Wrap(errGo, "could not open file "+name).With("stack", stack.Trace().TrimRuntime())
	}
	defer obj.Close()

	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {

		var inReader io.ReadCloser

		switch fileType {
		case "application/x-gzip", "application/zip":
			inReader, errGo = gzip.NewReader(obj)
		case "application/bzip2", "application/octet-stream":
			inReader = ioutil.NopCloser(bzip2.NewReader(obj))
		default:
			inReader = ioutil.NopCloser(obj)
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
				return warns, errors.Wrap(errGo).With("file", path).With("stack", stack.Trace().TrimRuntime())
			}

			_, errGo = io.Copy(file, tarReader)
			file.Close()
			if errGo != nil {
				return warns, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	} else {
		fn := filepath.Join(output, filepath.Base(name))
		f, errGo := os.Create(fn)
		if errGo != nil {
			return warns, errors.Wrap(errGo).With("outputFile", fn).With("stack", stack.Trace().TrimRuntime())
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		if _, errGo = io.Copy(outf, obj); errGo != nil {
			return warns, errors.Wrap(errGo).With("outputFile", fn).With("stack", stack.Trace().TrimRuntime())
		}
		outf.Flush()
	}
	return warns, nil
}

// Deposit is not a supported feature of local caching
//
func (s *localStorage) Deposit(src string, dest string, timeout time.Duration) (warns []errors.Error, err errors.Error) {
	return warns, errors.New("localized storage caches do not support write through saving of files").With("stack", stack.Trace().TrimRuntime())
}
