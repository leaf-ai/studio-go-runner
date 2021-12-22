// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bufio"
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/leaf-ai/go-service/pkg/network"

	"github.com/golang/protobuf/ptypes/wrappers"
	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
	
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

func procOutput(stopWriter chan struct{}, f *os.File, outC chan []byte, errC chan string) {

	outLine := []byte{}

	defer func() {
		if len(outLine) != 0 {
			f.WriteString(string(outLine))
		}
		f.Close()
	}()

	refresh := time.NewTicker(2 * time.Second)
	defer refresh.Stop()

	for {
		select {
		case <-refresh.C:
			if len(outLine) != 0 {
				f.WriteString(string(outLine))
				outLine = []byte{}
			}
		case <-stopWriter:
			return
		case r := <-outC:
			if len(r) != 0 {
				outLine = append(outLine, r...)
				if !bytes.Contains([]byte{'\n'}, r) {
					continue
				}
			}
			if len(outLine) != 0 {
				f.WriteString(string(outLine))
				outLine = []byte{}
			}
		case errLine := <-errC:
			if len(errLine) != 0 {
				f.WriteString(errLine + "\n")
			}
		}
	}
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

	outC := make(chan []byte)
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

	// Protect the err value when running multiple goroutines
	errCheck := sync.Mutex{}

	// This code connects the pipes being used by the golang exec command process to the channels that
	// will be used to bring the output into a single file
	waitOnIO := sync.WaitGroup{}
	waitOnIO.Add(2)

	go func() {
		defer waitOnIO.Done()

		time.Sleep(time.Second)

		responseLine := strings.Builder{}
		s := bufio.NewScanner(stdout)
		s.Split(bufio.ScanRunes)
		for s.Scan() {
			out := s.Bytes()
			outC <- out
			if bytes.Compare(out, []byte{'\n'}) == 0 {
				responseLine.Write(out)
			} else {
				if responseQ != nil && responseLine.Len() != 0 {
					select {
					case responseQ <- &runnerReports.Report{
						Time: timestamppb.Now(),
						ExecutorId: &wrappers.StringValue{
							Value: network.GetHostName(),
						},
						UniqueId: &wrappers.StringValue{
							Value: runID,
						},
						ExperimentId: &wrappers.StringValue{
							Value: runKey,
						},
						Payload: &runnerReports.Report_Logging{
							Logging: &runnerReports.LogEntry{
								Time:     timestamppb.Now(),
								Severity: runnerReports.LogSeverity_Info,
								Message: &wrappers.StringValue{
									Value: responseLine.String(),
								},
								Fields: map[string]string{},
							},
						},
					}:
						responseLine.Reset()
					default:
						// Dont respond to back pressure
					}
				}
			}
		}
		if errGo := s.Err(); errGo != nil {
			errCheck.Lock()
			defer errCheck.Unlock()
			if err != nil && err != os.ErrClosed {
				err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	go func() {
		defer waitOnIO.Done()

		time.Sleep(time.Second)
		s := bufio.NewScanner(stderr)
		s.Split(bufio.ScanLines)
		for s.Scan() {
			out := s.Text()
			errC <- out
			if responseQ != nil {
				select {
				case responseQ <- &runnerReports.Report{
					Time: timestamppb.Now(),
					ExecutorId: &wrappers.StringValue{
						Value: network.GetHostName(),
					},
					UniqueId: &wrappers.StringValue{
						Value: runID,
					},
					ExperimentId: &wrappers.StringValue{
						Value: runKey,
					},
					Payload: &runnerReports.Report_Logging{
						Logging: &runnerReports.LogEntry{
							Time:     timestamppb.Now(),
							Severity: runnerReports.LogSeverity_Error,
							Message: &wrappers.StringValue{
								Value: string(out),
							},
							Fields: map[string]string{},
						},
					},
				}:
				default:
					// Dont respond to back preassure
				}
			}
		}
		if errGo := s.Err(); errGo != nil {
			errCheck.Lock()
			defer errCheck.Unlock()
			if err != nil && err != os.ErrClosed {
				err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	// Wait for the IO to stop before continuing to tell the background
	// writer to terminate. This means the IO for the process will
	// be able to send on the channels until they have stopped.
	waitOnIO.Wait()

	// Now manually stop the process output copy goroutine once the exec package
	// has finished
	close(stopOutput)

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo = cmd.Wait(); errGo != nil {
		errCheck.Lock()
		if err == nil {
			err = kv.Wrap(errGo).With("loc", "cmd.Wait()").With("stack", stack.Trace().TrimRuntime())
		}
		errCheck.Unlock()
	}

	errCheck.Lock()
	if err == nil && stopCmd.Err() != nil {
		err = kv.Wrap(stopCmd.Err()).With("loc", "stopCmd").With("stack", stack.Trace().TrimRuntime())
	}
	errCheck.Unlock()

	return err
}
