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
		return nil, errGo.(errors.Error)
	}

	return t, nil
}

func (t *TarWriter) HasFiles() bool {
	return len(t.files) != 0
}

func (t *TarWriter) Write(tw *tar.Writer) (err errors.Error) {

	for file, header := range t.files {
		// write the header
		if errGo := tw.WriteHeader(header); errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		// return on directories since there will be no content to tar, only headers
		fi, err := os.Stat(file)
		if err != nil {
			return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		}

		if !fi.Mode().IsRegular() {
			continue
		}

		// open files for taring
		f, err := os.Open(file)
		if err != nil {
			return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		}

		// copy file data into tar writer
		if _, err := io.Copy(tw, f); err != nil {
			f.Close()
			return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		}
		f.Close()

	}
	return nil
}
