// Copyright 2020-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package mime // import "github.com/leaf-ai/go-service/pkg/mime"

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// DetectFileType can be used to examine the contexts of a file and return
// the most likely match for its contents as a mime type.
//
func DetectFileType(fn string) (typ string, err kv.Error) {
	file, errOs := os.Open(filepath.Clean(fn))
	if errOs != nil {
		return "", kv.Wrap(errOs).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer file.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)
	if _, errOs = file.Read(buffer); errOs != nil && errOs != io.EOF {
		return "", kv.Wrap(errOs).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	// Always returns a valid content-type and "application/octet-stream" if no others seemed to match.
	return http.DetectContentType(buffer), nil
}

// MimeFromExt is used to characterize a mime type from a files extension
//
func MimeFromExt(name string) (fileType string, err kv.Error) {
	switch filepath.Ext(name) {
	case ".gzip", ".gz":
		return "application/x-gzip", nil
	case ".zip":
		return "application/zip", nil
	case ".tgz": // Non standard extension as a result of studioml python code
		return "application/bzip2", nil
	case ".tb2", ".tbz", ".tbz2", ".bzip2", ".bz2": // Standard bzip2 extensions
		return "application/bzip2", nil
	case ".tar":
		return "application/tar", nil
	case ".bin":
		return "application/octet-stream", nil
	default:
		fileType, errGo := DetectFileType(name)
		if errGo != nil {
			// Fill in a default value even if there is an error
			return "application/octet-stream", errGo
		}
		return fileType, nil
	}
}
