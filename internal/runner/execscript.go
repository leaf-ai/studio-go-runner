// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
)

func writeOut(f *os.File, line string, tag string) {
	fmt.Printf("writing line |%s| to %s\n", line, tag)
	if len(line) == 0 {
		return
	}
	n, err := f.WriteString(line + "\n")
	if err != nil {
		fmt.Printf("ERROR writing line |%s| to %s: %s\n", line, tag, err.Error())
	} else {
		fmt.Printf("written %d bytes to %s\n", n, tag)
	}
}

func procOutput(stopWriter chan struct{}, f *os.File, outC chan string, errC chan string) {

	defer func() {
		f.Close()
	}()

	for {
		select {
		case <-stopWriter:
			fmt.Println("procOutput STOPPED")
			return
		case errLine := <-errC:
			writeOut(f, errLine, "err")
		case outLine := <-outC:
			writeOut(f, outLine, "out")
		}
	}
}

func readToChan(input io.ReadCloser, output chan string, waitOnIO *sync.WaitGroup, result *error, tag string) {
	defer waitOnIO.Done()

	time.Sleep(time.Second)
	s := bufio.NewScanner(input)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		out := s.Text()
		fmt.Printf("READ |%s| from %s\n", out, tag)
		output <- out
	}
	*result = s.Err()
}

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts.
//
func RunScript(ctx context.Context, scriptPath string, output *os.File,
	responseQ chan<- *runnerReports.Report, runKey string, runID string) (err kv.Error) {

	stopCmd, stopCmdCancel := context.WithCancel(context.Background())
	// defers are stacked in LIFO order so cancelling this context is the last
	// thing this function will do, also cancelling the stopCmd will also travel down
	// the context hierarchy cancelling everything else
	defer stopCmdCancel()

	// Cancel our own internal context when the outer context is cancelled
	go func() {
		select {
		case <-stopCmd.Done():
		case <-ctx.Done():
		}
		stopCmdCancel()
	}()

	// Create a new TMPDIR because the script python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", runKey)
	if errGo != nil {
		return kv.Wrap(errGo).With("experimentKey", runKey).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(tmpDir)

	// Move to starting the process that we will monitor
	// #nosec
	cmd := exec.CommandContext(stopCmd, "/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+filepath.Clean(scriptPath))
	cmd.Dir = path.Dir(scriptPath)

	// Pipes are used to allow the output to be tracked interactively from the cmd
	stdout, errGo := cmd.StdoutPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	stderr, errGo := cmd.StderrPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	outC := make(chan string)
	defer close(outC)
	errC := make(chan string)
	defer close(errC)

	// A quit channel is used to allow fine grained control over when the IO
	// copy and output task should be created
	stopOutput := make(chan struct{}, 1)

	// Being the go routine that takes cmd output and appends it to a file on disk
	go procOutput(stopOutput, output, outC, errC)

	// Start begins the processing asynchronously, the procOutput above will collect the
	// run results are they are output asynchronously
	if errGo = cmd.Start(); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// This code connects the pipes being used by the golang exec command process to the channels that
	// will be used to bring the output into a single file
	waitOnIO := sync.WaitGroup{}
	waitOnIO.Add(2)

	var errStdOut error
	var errErrOut error

	go readToChan(stdout, outC, &waitOnIO, &errStdOut, "err")
	go readToChan(stderr, errC, &waitOnIO, &errErrOut, "out")

	// Wait for the IO to stop before continuing to tell the background
	// writer to terminate. This means the IO for the process will
	// be able to send on the channels until they have stopped.
	waitOnIO.Wait()

	// Now manually stop the process output copy goroutine once the exec package
	// has finished
	close(stopOutput)

	if errStdOut != nil {
		if err == nil || err == os.ErrClosed {
			err = kv.Wrap(errStdOut).With("stack", stack.Trace().TrimRuntime())
		}
	}
	if errErrOut != nil {
		if err == nil || err == os.ErrClosed {
			err = kv.Wrap(errErrOut).With("stack", stack.Trace().TrimRuntime())
		}
	}

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo = cmd.Wait(); errGo != nil {
		if err == nil {
			err = kv.Wrap(errGo).With("loc", "cmd.Wait()").With("stack", stack.Trace().TrimRuntime())
		}
	}

	if err == nil && stopCmd.Err() != nil {
		err = kv.Wrap(stopCmd.Err()).With("loc", "stopCmd").With("stack", stack.Trace().TrimRuntime())
	}

	return err
}
