// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation for the storage sub system that will
// be used by the runner to retrieve storage from local storage

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/leaf-ai/go-service/pkg/mime"
	"github.com/leaf-ai/studio-go-runner/internal/defense"

	"github.com/go-stack/stack"

	"github.com/jjeffery/kv" // MIT License
)

type localStorage struct {
}

// NewLocalStorage is used to allocate and initialize a struct that acts as a receiver
//
func NewLocalStorage() (s *localStorage, err kv.Error) {
	return &localStorage{}, nil
}

// Close is a NoP unless overridden
func (s *localStorage) Close() {
}

// Hash returns a platform specific hash of the contents of the file that can be used by caching and other functions
// to track storage changes etc
//
func (s *localStorage) Hash(ctx context.Context, name string) (hash string, err kv.Error) {
	return filepath.Base(name), nil
}

// Gather is used to retrieve files prefixed with a specific key.  It is used to retrieve the individual files
// associated with a previous Hoard operation
//
func (s *localStorage) Gather(ctx context.Context, keyPrefix string, outputDir string, maxBytes int64, tap io.Writer, failFast bool) (size int64, warnings []kv.Error, err kv.Error) {
	return 0, warnings, kv.NewError("unimplemented").With("stack", stack.Trace().TrimRuntime())
}

// Fetch is used to retrieve a file from a well known disk directory and either
// copy it directly into a directory, or unpack the file into the same directory.
//
// Calling this function with output not being a valid directory will result in an error
// being returned.
//
// The tap can be used to make a side copy of the content that is being read.
//
func (s *localStorage) Fetch(ctx context.Context, name string, unpack bool, output string, maxBytes int64, tap io.Writer) (size int64, warns []kv.Error, err kv.Error) {

	kv := kv.With("output", output).With("name", name)

	// Make sure output is an existing directory
	info, errGo := os.Stat(output)
	if errGo != nil {
		return 0, warns, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !info.IsDir() {
		return 0, warns, kv.NewError(output+" is not a directory").With("stack", stack.Trace().TrimRuntime())
	}

	fileType, err := mime.MimeFromExt(name)
	if err != nil {
		warns = append(warns, kv.Wrap(err).With("fn", name).With("type", fileType).With("stack", stack.Trace().TrimRuntime()))
	} else {
		warns = append(warns, kv.NewError("debug").With("fn", name).With("type", fileType).With("stack", stack.Trace().TrimRuntime()))
	}

	obj, errGo := os.Open(filepath.Clean(name))
	if errGo != nil {
		return 0, warns, kv.Wrap(errGo, "could not open file "+name).With("stack", stack.Trace().TrimRuntime())
	}
	defer obj.Close()

	return fetcher(obj, name, output, maxBytes, fileType, unpack)
}

func addReader(obj *os.File, fileType string) (inReader io.ReadCloser, err kv.Error) {
	switch fileType {
	case "application/x-gzip", "application/zip":
		reader, errGo := gzip.NewReader(obj)
		if errGo != nil {
			return reader, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		inReader = reader
	case "application/bzip2", "application/octet-stream":
		inReader = ioutil.NopCloser(bzip2.NewReader(obj))
	default:
		inReader = ioutil.NopCloser(obj)
	}
	return inReader, err
}

func fetcher(obj *os.File, name string, output string, maxBytes int64, fileType string, unpack bool) (size int64, warns []kv.Error, err kv.Error) {
	// If the unpack flag is set then use a tar decompressor and unpacker
	// but first make sure the output location is an existing directory
	if unpack {

		inReader, err := addReader(obj, fileType)
		if err != nil {
			return 0, warns, err
		}
		defer inReader.Close()

		tarReader := tar.NewReader(inReader)

		for {
			header, errGo := tarReader.Next()
			if errors.Is(errGo, io.EOF) {
				break
			} else if errGo != nil {
				return 0, warns, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			if escapes, err := defense.WillEscape(header.Name, output); escapes {
				if err != nil {
					return 0, warns, kv.Wrap(err).With("filename", header.Name, "output", output)
				} else {
					return 0, warns, kv.NewError("archive escaped").With("filename", header.Name, "output", output)
				}
			}

			path, _ := filepath.Abs(filepath.Join(output, header.Name))
			if !strings.HasPrefix(path, output) {
				return 0, warns, kv.NewError("archive file name escaped").With("filename", header.Name).With("stack", stack.Trace().TrimRuntime())
			}

			info := header.FileInfo()
			if info.IsDir() {
				if errGo = os.MkdirAll(path, info.Mode()); errGo != nil {
					return 0, warns, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
				}
				continue
			}

			file, errGo := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
			if errGo != nil {
				return 0, warns, kv.Wrap(errGo).With("file", path).With("stack", stack.Trace().TrimRuntime())
			}

			size, errGo = io.CopyN(file, tarReader, maxBytes)
			file.Close()
			if errGo != nil {
				if !errors.Is(errGo, io.EOF) {
					return 0, warns, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
				}
				errGo = nil
			}
		}
	} else {
		fn := filepath.Join(output, filepath.Base(name))
		f, errGo := os.Create(fn)
		if errGo != nil {
			return 0, warns, kv.Wrap(errGo).With("outputFile", fn).With("stack", stack.Trace().TrimRuntime())
		}
		defer f.Close()

		outf := bufio.NewWriter(f)
		size, errGo = io.CopyN(outf, obj, maxBytes)
		if errGo != nil {
			if !errors.Is(errGo, io.EOF) {
				return 0, warns, kv.Wrap(errGo).With("outputFile", fn).With("stack", stack.Trace().TrimRuntime())
			}
			errGo = nil
		}
		outf.Flush()
	}
	return size, warns, nil
}

// Hoard is not a supported feature of local caching
//
func (s *localStorage) Hoard(ctx context.Context, src string, destPrefix string) (warns []kv.Error, err kv.Error) {
	return warns, kv.NewError("localized storage caches do not support write through saving of files").With("stack", stack.Trace().TrimRuntime())
}

// Deposit is not a supported feature of local caching
//
func (s *localStorage) Deposit(ctx context.Context, src string, dest string) (warns []kv.Error, err kv.Error) {
	return warns, kv.NewError("localized storage caches do not support write through saving of files").With("stack", stack.Trace().TrimRuntime())
}
