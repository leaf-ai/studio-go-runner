// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"syscall"
)

// Run will use a generated script file and will run it to completion while marshalling
// results and files from the computation.  Run is a blocking call and will only return
// upon completion or termination of the process it starts.
//
func RunScript(ctx context.Context, scriptPath string, output *os.File, tmpDir string,
	runKey string, logger *log.Logger) (err kv.Error) {

	defer func() {
		errMsg := "none"
		if err != nil {
			errMsg = err.Error()
		}
		logger.Info("EXITING RunScript", "runKey", runKey, "error:", errMsg)
	}()

	stopCmd, origCancel := context.WithCancel(context.Background())
	stopCmdCancel := GetCancelWrapper(origCancel, "bash script context", logger)
	// defers are stacked in LIFO order so cancelling this context is the last
	// thing this function will do, also cancelling the stopCmd will also travel down
	// the context hierarchy cancelling everything else
	defer stopCmdCancel()

	defer func() {
		if "" != tmpDir {
			os.RemoveAll(tmpDir)
		}
	}()

	// Move to starting the process that we will monitor
	// #nosec
	cmd := exec.Command(filepath.Clean(scriptPath))
	cmd.Dir = path.Dir(scriptPath)

	logFilter := GetLogFilterer()
	logWriter := GetFilteredOutputWriter(output, logger, logFilter)
	stdOut, stdErr := logWriter.GetWriters()

	cmd.Stdout = stdOut
	cmd.Stderr = stdErr
	//cmd.Stdout = output
	//cmd.Stderr = output

	// Cancel our own internal context when the outer context is cancelled
	go func() {
		select {
		case <-stopCmd.Done():
			logger.Debug("RunScript: cmd context cancelled", "stack", stack.Trace().TrimRuntime())
		case <-ctx.Done():
			logger.Debug("RunScript: outer context cancelled", "stack", stack.Trace().TrimRuntime())
			if errGo := cmd.Process.Signal(syscall.SIGHUP); errGo != nil {
				err = kv.Wrap(errGo).With("key", runKey).With("stack", stack.Trace().TrimRuntime())
				logger.Debug("RunScript: failed to send signal to workload process", "error", err.Error())
			} else {
				logger.Debug("RunScript: signal sent to workload process", "key", runKey, "stack", stack.Trace().TrimRuntime())
			}
			stopCmdCancel()
		}
	}()

	// Start begins the processing asynchronously, the procOutput above will collect the
	// run results are they are output asynchronously
	if errGo := cmd.Start(); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo := cmd.Wait(); errGo != nil {
		if err == nil {
			err = kv.Wrap(errGo).With("loc", "cmd.Wait()").With("stack", stack.Trace().TrimRuntime())
		}
	}

	if err == nil && stopCmd.Err() != nil {
		err = kv.Wrap(stopCmd.Err()).With("loc", "stopCmd").With("stack", stack.Trace().TrimRuntime())
	}

	return err
}
