// Copyright 2018-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package archive // import "github.com/leaf-ai/go-service/pkg/archive"

// This file contains implementations of some tar handling functions and methods to add a little
// structure around tar file handling when specifically writing files into archives on streaming
// devices or file systems

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// TarWriter encapsulates a writer of tar files that stores the source dir and the headers that
// will be used to generate a studioml artifact
type TarWriter struct {
	dir   string
	files map[string]*tar.Header
}

// IsTar is used to test the extension to see if the presence of tar can be found
//
func IsTar(name string) bool {
	switch {
	case strings.Contains(name, ".tar."):
		return true
	case strings.HasSuffix(name, ".tgz"):
		return true
	case strings.HasSuffix(name, ".tar"):
		return true
	case strings.HasSuffix(name, ".tar.bzip2"):
		return true
	case strings.HasSuffix(name, ".tar.bz2"):
		return true
	case strings.HasSuffix(name, ".tbz2"):
		return true
	case strings.HasSuffix(name, ".tbz"):
		return true
	}
	return false
}

// NewTarWriter generates a data structure to encapsulate the tar headers for the
// files within a caller specified directory that can be used to generate an artifact
//
func NewTarWriter(dir string) (t *TarWriter, err kv.Error) {

	t = &TarWriter{
		dir:   dir,
		files: map[string]*tar.Header{},
	}

	errGo := filepath.Walk(dir, func(file string, fi os.FileInfo, err error) error {

		// return on any error
		if err != nil {
			return err
		}

		link := ""
		if fi.Mode()&os.ModeSymlink == os.ModeSymlink {
			if link, err = os.Readlink(file); err != nil {
				return kv.Wrap(err).With("stack", stack.Trace().TrimRuntime())
			}
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return kv.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		}

		// update the name to correctly reflect the desired destination when untaring
		header.Name = strings.TrimPrefix(strings.Replace(file, dir, "", -1), string(filepath.Separator))

		if len(header.Name) == 0 {
			// Our output directory proper, ignore it
			return nil
		}

		t.files[file] = header

		return nil
	})

	if errGo != nil {
		err, ok := errGo.(kv.Error)
		if ok {
			return nil, err
		}
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return t, nil
}

// HasFiles is used to test the artifact file catalog to see if there are files
// within it
//
func (t *TarWriter) HasFiles() bool {
	return len(t.files) != 0
}

// Write is used to add a go tar file writer device to the
// tar writer and to output the files within the catalog of the
// runners file list into the go tar device
//
func (t *TarWriter) Write(tw *tar.Writer) (err kv.Error) {

	for file, header := range t.files {
		err = func() (err kv.Error) {
			// return on directories since there will be no content to tar, only headers
			fi, errGo := os.Stat(file)
			if errGo != nil {
				// Working files can be recycled on occasion and disappear, handle this
				// possibility
				if os.IsNotExist(errGo) {
					return nil
				}
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}

			// open files for taring, skip files that could not be opened, this could be due to working
			// files getting scratched etc and is legal
			f, errGo := os.Open(filepath.Clean(file))
			if errGo != nil {
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}
			defer func() { _ = f.Close() }()

			// write the header
			if errGo := tw.WriteHeader(header); errGo != nil {
				return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}

			if !fi.Mode().IsRegular() {
				return nil
			}

			// copy file data into tar writer
			if _, err := io.CopyN(tw, f, header.Size); err != nil {
				return kv.Wrap(err).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
