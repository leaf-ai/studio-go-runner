// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"io"
	"os"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the implementation of a reverse file reader that can be used with
// bufio scanners
//
//
//	rf := &ReverseFile{
//			File: aFile,
//         }
//	if err := rf.SeekEnd(); err != nil {
//      return err
//  }
//
//	s := bufio.NewScanner(rr)
//	for s.Scan() {
//		fmt.Println(s.Text())
//	}
//
//				responseLog, err := runner.NewReverseReader(filepath.Join(tmpDir, "responses"))
//				if err != nil {
//					logger.Info("reverse file read failed", "error", err.Error())
//					return
//				}
//				defer responseLog.Close()
//
//				s := bufio.NewScanner(responseLog)
//				for i := 0; s.Scan() && i < 10; i++ {
//					logger.Debug(runner.Reverse(s.Text()))
//				}

func NewReverseReader(fn string) (reader *ReverseReader, err kv.Error) {
	reader = &ReverseReader{}
	readerFile, errGo := os.Open(fn)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	reader.file = readerFile
	if errGo = reader.SeekEnd(); errGo != nil {
		readerFile.Close()
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return reader, nil
}

type ReverseReader struct {
	file *os.File
}

// Closer interface method
func (r *ReverseReader) Close() (err error) {
	return r.file.Close()
}

// Seek to the last byte of the file ready for reverse reading
func (r *ReverseReader) SeekEnd() (err kv.Error) {
	if _, errGo := r.file.Seek(0, io.SeekEnd); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// Read the file backwards
func (r *ReverseReader) Read(b []byte) (byteCnt int, err error) {
	if len(b) == 0 {
		return 0, nil
	}

	// This no-op gives us the current offset value
	offset, err := r.file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}

	for i := 0; i < len(b); i++ {
		if offset == 0 {
			return byteCnt, io.EOF
		}
		// Seek in case someone else is relying on seek too
		offset, err = r.file.Seek(-1, io.SeekCurrent)
		if err != nil {
			return byteCnt, err // Should never happen
		}

		// Just read one byte at a time
		n, err := r.file.ReadAt(b[i:i+1], offset)
		if err != nil {
			return byteCnt + n, err // Should never happen
		}
		byteCnt += n
	}
	return byteCnt, nil
}
