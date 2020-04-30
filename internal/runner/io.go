// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains routines for performing file io

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/karlmutch/circbuf"
	"github.com/karlmutch/vtclean"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// ReadLast will extract the last portion of data from a file up to a maximum specified by
// the caller.
//
func ReadLast(fn string, max uint32) (data string, err kv.Error) {
	file, errOs := os.Open(filepath.Clean(fn))
	if errOs != nil {
		return "", kv.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer file.Close()

	fi, errOs := file.Stat()
	if errOs != nil {
		return "", kv.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
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
		return "", kv.Wrap(errOs, fn).With("stack", stack.Trace().TrimRuntime())
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

// CopyFile is a simple file copy that will overwrite any destination
//
func CopyFile(srcFN string, dstFN string) (n int64, err kv.Error) {
	stat, errGo := os.Stat(srcFN)
	if errGo != nil {
		return 0, kv.Wrap(errGo).With("source", src).With("stack", stack.Trace().TrimRuntime())
	}

	if !stat.Mode().IsRegular() {
		return 0, kv.NewError("not a regular file").With("source", src).With("stack", stack.Trace().TrimRuntime())
	}

	src, errGo := os.Open(srcFN)
	if errGo != nil {
		return 0, kv.Wrap(errGo).With("source", srcFN).With("stack", stack.Trace().TrimRuntime())
	}
	defer src.Close()

	dst, errGo := os.Create(dstFN)
	if err != nil {
		return 0, kv.Wrap(errGo).With("dst", dstFN).With("stack", stack.Trace().TrimRuntime())
	}
	defer dst.Close()

	if n, errGo = io.Copy(dst, src); errGo != nil {
		return 0, kv.Wrap(errGo).With("source", src, "dst", dstFN).With("stack", stack.Trace().TrimRuntime())
	}
	return n, nil
}
