package runner

// This file contains routines for performing file io

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"os"

	"github.com/karlmutch/circbuf"
	"github.com/karlmutch/vtclean"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// ReadLast will extract the last portion of data from a file up to a maximum specified by
// the caller.
//
func ReadLast(fn string, max uint32) (data string, err errors.Error) {
	file, errOs := os.Open(fn)
	if errOs != nil {
		return "", errors.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer file.Close()

	fi, errOs := file.Stat()
	if errOs != nil {
		return "", errors.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
	}

	// Suck up a lot of data to allow us to process lines with backspaces etc and still be left with
	// something useful
	//
	buf := make([]byte, 1024*1024)
	readStart := fi.Size() - int64(len(buf))

	if readStart <= 0 {
		readStart = 0
	}

	n, errOs := file.ReadAt(buf, readStart)
	if errOs != nil && errOs != io.EOF {
		return "", errors.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
	}

	ring, _ := circbuf.NewBuffer(int64(max))
	s := bufio.NewScanner(bytes.NewReader(buf[:n]))
	for s.Scan() {
		ring.Write([]byte(vtclean.Clean(s.Text(), true)))
		ring.Write([]byte{'\n'})
	}
	return string(ring.Bytes()), nil
}

// DetectFileType can be used to examine the contexts of a file and return
// the most likely match for its contents as a mime type.
//
func DetectFileType(fn string) (typ string, err errors.Error) {
	file, errOs := os.Open(fn)
	if errOs != nil {
		return "", errors.Wrap(errOs).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer file.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)
	if _, errOs = file.Read(buffer); errOs != nil && errOs != io.EOF {
		return "", errors.Wrap(errOs).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	// Always returns a valid content-type and "application/octet-stream" if no others seemed to match.
	return http.DetectContentType(buffer), nil
}
