package runner

// This file contains implementations of some tar handling functions

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

func TarGetFiles(dir string) (found map[string]*tar.Header, err errors.Error) {

	found = map[string]*tar.Header{}

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

		found[file] = header

		return nil
	})

	if errGo != nil {
		return nil, errGo.(errors.Error)
	}

	return found, nil
}
