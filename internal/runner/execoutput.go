// Copyright 2018-2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"github.com/andreidenissov-cog/go-service/pkg/log"
	"io"
	"os"
	"sync"
	"unicode/utf8"
)

type LogOutputProvider interface {
	GetWriters() (io.Writer, io.Writer)
	Close() error
}

var (
	initBufSize = 1024 * 1024
	addBufSize  = 512 * 1024
)

type OutputWriter struct {
	output *os.File
	filter func(string) string
	logger *log.Logger
	bwOne  *BufferedWriter
	bwTwo  *BufferedWriter
	sync.Mutex
}

func GetFilteredOutputWriter(externOut *os.File, logger *log.Logger, filter func(string) string) LogOutputProvider {
	filterOutput := &OutputWriter{}
	filterOutput.init(externOut, logger)
	filterOutput.setFilter(filter)
	return filterOutput
}

func (or *OutputWriter) init(externOut *os.File, logger *log.Logger) {
	or.output = externOut
	or.logger = logger
	or.filter = nil
	or.bwOne = &BufferedWriter{}
	or.bwOne.init("stdout", or)
	or.bwTwo = &BufferedWriter{}
	or.bwTwo.init("stderr", or)
}

func (or *OutputWriter) setFilter(f func(string) string) {
	or.filter = f
}

func (or *OutputWriter) GetWriters() (io.Writer, io.Writer) {
	return or.bwOne, or.bwTwo
}

func (or *OutputWriter) Close() error {
	or.bwOne.Close()
	or.bwTwo.Close()
	return nil
}

func (or *OutputWriter) sendOut(name string, line []byte) {
	or.Lock()
	defer or.Unlock()

	_, err := or.output.Write(line)
	if err != nil {
		or.logger.Info("Error writing log output", "log:", name, "err:", err.Error())
	}
}

type BufferedWriter struct {
	name       string
	lineBuf    []byte
	startRunes int
	endRunes   int
	endBytes   int
	host       *OutputWriter
}

func (wr *BufferedWriter) init(name string, host *OutputWriter) {
	wr.lineBuf = make([]byte, initBufSize)
	wr.startRunes = 0
	wr.endRunes = 0
	wr.endBytes = 0
	wr.host = host
	wr.name = name
}

func (wr *BufferedWriter) extend(req int) {
	avail := len(wr.lineBuf)
	if avail >= req {
		return
	}
	for avail < req {
		avail = avail + addBufSize
	}
	newBuf := make([]byte, avail)
	copy(newBuf, wr.lineBuf[:wr.endBytes])
	wr.lineBuf = newBuf
}

func (wr *BufferedWriter) Write(p []byte) (n int, err error) {
	if p == nil || len(p) == 0 {
		return 0, nil
	}
	n = len(p)
	req := wr.endBytes + len(p) + 8
	// Ensure we have enough space in the buffer:
	wr.extend(req)
	copy(wr.lineBuf[wr.endBytes:], p)
	wr.endBytes += len(p)

	// Scan finished text lines in lineBuf and send them out:
	for wr.scan() {
		// We have line in lineBuf[startRunes:endRunes]
		wr.host.sendOut(wr.name, wr.lineBuf[wr.startRunes:wr.endRunes])
		wr.startRunes = wr.endRunes
	}
	// Clear out bytes we have written:
	if wr.startRunes > 0 {
		copy(wr.lineBuf, wr.lineBuf[wr.startRunes:wr.endBytes])
		wr.endRunes -= wr.startRunes
		wr.endBytes -= wr.startRunes
		wr.startRunes = 0
	}
	return n, nil
}

func (wr *BufferedWriter) scan() bool {
	for wr.endRunes < wr.endBytes {
		// Try to advance one rune in an already read data:
		if wr.lineBuf[wr.endRunes] < utf8.RuneSelf {
			// fast path: simple ASCII
			if wr.lineBuf[wr.endRunes] == '\n' {
				// end-of-line found, yay
				wr.endRunes++
				return true
			}
			wr.endRunes++
			continue
		}
		r, rlen := utf8.DecodeRune(wr.lineBuf[wr.endRunes:wr.endBytes])
		if r != utf8.RuneError {
			// and we know it's not end-of-line:
			wr.endRunes += rlen
			continue
		}
		// we have a leftover incomplete rune in [endRunes:endData] bytes.
		return false
	}
	return false
}

func (wr *BufferedWriter) Close() error {
	wr.lineBuf[wr.endRunes] = '\n'
	wr.endRunes++
	wr.host.sendOut(wr.name, wr.lineBuf[wr.startRunes:wr.endRunes])
	return nil
}
