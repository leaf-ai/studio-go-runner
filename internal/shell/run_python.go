// Copyright 2020-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package shell

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/leaf-ai/studio-go-runner/internal/io"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// PythonRun will use the set of test files to start a python process that will return
// the console output and/or an error.  The caller can specify the max size of the buffer
// used to hold the last (keepLines) of lines from the console.  If tmpDir is specified
// that directory will be used to run the process and will not be removed after the
// function completes, in the case it is blank then the function will generate a directory
// run the python in it then remove it.
//
func PythonRun(testFiles map[string]os.FileMode, tmpDir string, script string, keepLines uint) (output []string, err kv.Error) {

	output = []string{}

	// If the optional temporary directory is not supplied then we create,
	// use it and then remove it, this allows callers to load data into the
	// directory they supply if they wish
	if len(tmpDir) == 0 {
		// Create a new TMPDIR because the python pip tends to leave dirt behind
		// when doing pip builds etc
		t, errGo := ioutil.TempDir("", "")
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		defer func() {
			os.RemoveAll(t)
		}()
		tmpDir = t
	}

	for fn, mode := range testFiles {
		assetFN, errGo := filepath.Abs(fn)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		destFN, errGo := filepath.Abs(filepath.Join(tmpDir, filepath.Base(assetFN)))
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		if _, err := io.CopyFile(assetFN, destFN); err != nil {
			return nil, err
		}

		if errGo = os.Chmod(destFN, mode); errGo != nil {
			return nil, kv.Wrap(errGo).With("destFN", destFN, "stack", stack.Trace().TrimRuntime())
		}
	}

	// Look for a single script file if the script parameter is not supplied
	if len(script) == 0 {
		matches, errGo := filepath.Glob(filepath.Join(tmpDir, "*.sh"))
		if len(matches) == 0 {
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			return nil, kv.NewError("no script files found").With("stack", stack.Trace().TrimRuntime())
		}
		if len(matches) > 1 {
			return nil, kv.NewError("too many script files found").With("stack", stack.Trace().TrimRuntime())
		}
		script = matches[0]
	}

	if len(script) == 0 {
		return nil, kv.NewError("script not found or specified").With("stack", stack.Trace().TrimRuntime())
	}

	// Save the output from the run using the last say 10 lines as a default otherwise
	// use the callers specified number of lines if they specified any
	if keepLines == 0 {
		keepLines = 20
	}
	output = make([]string, 0, keepLines)

	// Now setup is done execute the experiment
	dataC := make(chan *string, 1)
	go func() {
		for {
			select {
			case line := <-dataC:
				if line == nil {
					return
				}
				// Push to the back of the stack of lines, then pop from the front
				output = append(output, *line)
				if len(output) > int(keepLines) {
					output = output[1:]
				}
			}
		}
	}()

	// When running python change directory into the temporary sandbox we are using
	originalDir, errGo := os.Getwd()
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if tmpDir != originalDir {
		if errGo = os.Chdir(tmpDir); errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		// Once the python run is done we jump back to the original directory
		defer os.Chdir(originalDir)
	}

	return output, CmdRun(context.TODO(), script, dataC)
}
