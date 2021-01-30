// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leaf-ai/studio-go-runner/internal/shell"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// TestOpenBlas is used to validate that openblas is available to our python
// base installation
//
func TestOpenBlas(t *testing.T) {
	// Create a new TMPDIR because the python pip tends to leave dirt behind
	// when doing pip builds etc
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	// Grab know files from the crypto test library and place them into
	// our temporary test directory
	testFiles := map[string]os.FileMode{
		filepath.Join("..", "..", "assets", "openblas", "openblas.py"): 0600,
		filepath.Join("..", "..", "assets", "openblas", "openblas.sh"): 0700,
	}

	output, err := shell.PythonRun(testFiles, "", "", 128)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range output {
		if strings.Contains(line, "libraries = ['openblas', 'openblas']") {
			return
		}
	}
	t.Fatal(kv.NewError("Openblas not detected").With("lines", output, "stack", stack.Trace().TrimRuntime()))
}
