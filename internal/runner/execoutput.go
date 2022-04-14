// Copyright 2022 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"github.com/jjeffery/kv"
	"io"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/andreidenissov-cog/go-service/pkg/log"
)

var (
	bufferSize   = 16 * 1024
	endThreshold = 32
)

type LockableWriter interface {
	io.Writer
	sync.Locker
}

type streamBuffer struct {
	data       []byte
	startRunes int
	endRunes   int
	endData    int
	next       *streamBuffer
}

type StreamHandler struct {
	input       io.Reader
	inputId     string
	output      LockableWriter
	outputId    string
	first       *streamBuffer
	last        *streamBuffer
	freeBuffers *streamBuffer
	isDone      bool
	errs        []kv.Error
}

func GetStreamHandler(input io.Reader, inputId string, output LockableWriter, outputId string) *StreamHandler {
	handler := &StreamHandler{
		input:       input,
		inputId:     inputId,
		output:      output,
		outputId:    outputId,
		isDone:      false,
		freeBuffers: nil,
		first:       nil,
		last:        nil,
		errs:        []kv.Error{},
	}
	handler.addBuffer([]byte{})
	return handler
}

func (sh *StreamHandler) stream(wg *sync.WaitGroup) {
	defer wg.Done()

	sh.isDone = false
	for !sh.isDone {
		cap := len(sh.last.data) - sh.last.endData
		if cap < endThreshold {
			// too little space in current buffer, add another one
			current := sh.last
			sh.addBuffer(current.data[current.endRunes:current.endData])
		}
		sh.isDone = sh.last.read(sh.input, sh.inputId, sh.logger)
		// We read in some new input, now scan for ALL finished lines
		// that we see and send them out.
		for sh.last.scan() {
			// we have next full line
			sh.write()
		}
	}
	// write out whatever is left
	sh.write()
	sh.close()
}

func (sh *StreamHandler) addBuffer(head []byte) {
	var newBuf *streamBuffer = nil
	if sh.freeBuffers != nil {
		newBuf = sh.freeBuffers
		sh.freeBuffers = sh.freeBuffers.next
		newBuf.startRunes = 0
		newBuf.endRunes = 0
		newBuf.endData = 0
	} else {
		newBuf = &streamBuffer{
			data:       make([]byte, bufferSize),
			startRunes: 0,
			endRunes:   0,
			endData:    0,
		}
	}
	newBuf.next = nil
	if sh.first == nil {
		sh.first = newBuf
	} else {
		sh.last.next = newBuf
	}
	sh.last = newBuf
	for i, b := range head {
		newBuf.data[i] = b
	}
	newBuf.endData = len(head)
}

func (sh *StreamHandler) close() {
	sh.first = nil
	sh.last = nil
	sh.freeBuffers = nil
}

func (sh *StreamHandler) releaseBuffer(buf *streamBuffer) {
	// reset buffer to empty
	buf.startRunes = 0
	buf.endRunes = 0
	buf.endData = 0
	// and return it to collection of free available buffers
	buf.next = sh.freeBuffers
	sh.freeBuffers = buf
}

func (sh *StreamHandler) write() {
	sh.output.Lock()
	defer sh.output.Unlock()

	for {
		_, err := sh.output.Write(sh.first.data[sh.first.startRunes:sh.first.endRunes])
		if err != nil {
			sh.logger.Error("error writing output", "id", sh.outputId, "err", err.Error())
		}
		if sh.first == sh.last {
			sh.first.startRunes = sh.first.endRunes
			return
		} else {
			next := sh.first.next
			sh.releaseBuffer(sh.first)
			sh.first = next
		}
	}
}

func (sb *streamBuffer) scan() bool {
	for sb.endRunes < sb.endData {
		// Try to advance one rune in an already read data:
		if sb.data[sb.endRunes] < utf8.RuneSelf {
			// fast path: simple ASCII
			if sb.data[sb.endRunes] == '\n' {
				// end-of-line found, yay
				sb.endRunes++
				return true
			}
			sb.endRunes++
			continue
		}
		r, rlen := utf8.DecodeRune(sb.data[sb.endRunes:sb.endData])
		if r != utf8.RuneError {
			// and we know it's not end-of-line:
			sb.endRunes += rlen
			continue
		}
		// we have a leftover incomplete rune in [endRunes:endData] bytes.
		return false
	}
	return false
}

func (sb *streamBuffer) read(input io.Reader, inputId string, logger log.Logger) bool {
	for {
		n, err := input.Read(sb.data[sb.endData:])
		sb.endData += n
		if err == io.EOF {
			return true
		}
		if err != nil {
			logger.Error("error reading input", "id", inputId, "err", err.Error())
			return true
		}
		if n > 0 {
			return false
		}
		time.Sleep(2 * time.Second)
	}
}
