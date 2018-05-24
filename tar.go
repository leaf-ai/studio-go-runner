package runner

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
	"github.com/karlmutch/errors"
)

type TarWriter struct {
	dir   string
	files map[string]*tar.Header
}

func NewTarWriter(dir string) (t *TarWriter, err errors.Error) {

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
				return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
			}
		}

		// create a new dir/file header
		header, err := tar.FileInfoHeader(fi, link)
		if err != nil {
			return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
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
		err, ok := errGo.(errors.Error)
		if ok {
			return nil, err
		} else {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	return t, nil
}

func (t *TarWriter) HasFiles() bool {
	return len(t.files) != 0
}

func (t *TarWriter) Write(tw *tar.Writer) (err errors.Error) {

	for file, header := range t.files {
		err = func() (err errors.Error) {
			// return on directories since there will be no content to tar, only headers
			fi, errGo := os.Stat(file)
			if errGo != nil {
				// Working files can be recycled on occasion and disappear, handle this
				// possibility
				if os.IsNotExist(errGo) {
					return nil
				}
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}

			// open files for taring, skip files that could not be opened, this could be due to working
			// files getting scratched etc and is legal
			f, errGo := os.Open(file)
			if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}
			defer f.Close()

			// write the header
			if errGo := tw.WriteHeader(header); errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}

			if !fi.Mode().IsRegular() {
				return nil
			}

			// copy file data into tar writer
			if _, err := io.Copy(tw, f); err != nil {
				return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	return nil
}
