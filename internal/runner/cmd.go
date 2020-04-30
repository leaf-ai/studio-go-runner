// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"bufio"
	"context"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// CmdRun contains a function that will run a bash script without argument and will pass results back using
// the channel supplied by the caller.  Exit codes will be communicated via the err return.  The output
// channel will be closed on completion.
//
func CmdRun(ctx context.Context, bashScript string, output chan *string) (err kv.Error) {

	script := filepath.Clean(bashScript)
	defer close(output)

	if _, errGo := os.Stat(script); os.IsNotExist(errGo) {
		return kv.Wrap(errGo).With("script", script, "stack", stack.Trace().TrimRuntime())
	}

	// Create a new TMPDIR because the python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(tmpDir)

	// Move to starting the process that we will monitor with the experiment running within
	// it

	// #nosec
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", "export TMPDIR="+tmpDir+"; "+filepath.Clean(script))
	cmd.Dir = path.Dir(script)

	stdout, errGo := cmd.StdoutPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	stderr, errGo := cmd.StderrPipe()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = cmd.Start(); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	merged := io.MultiReader(stderr, stdout)
	scanner := bufio.NewScanner(merged)
	for scanner.Scan() {
		aLine := scanner.Text()[:]
		select {
		case output <- &aLine:
			continue
		case <-time.After(time.Second):
			continue
		}
	}

	// Wait for the process to exit, and store any error code if possible
	// before we continue to wait on the processes output devices finishing
	if errGo = cmd.Wait(); errGo != nil {
		if err == nil {
			err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	return err
}
