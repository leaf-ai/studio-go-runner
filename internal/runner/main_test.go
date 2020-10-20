// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/karlmutch/envflag"
)

var (
	useGPU = flag.Bool("no-gpu", false, "Used to skip test and other initialization GPU hardware code") //nolint
	useK8s = flag.Bool("use-k8s", false, "Used to enable test and other initialization for the Kubernetes cluster support")
	topDir = flag.String("top-dir", "../..", "The location of the top level source directory for locating test files")

	// TestOptions are externally visible symbols that this package is asking the unit test suite to pickup and use
	// when the testing is managed by an external entity, this allows build level variations that include or
	// exclude GPUs for example to run their tests appropriately.  It also allows the top level build logic
	// to inspect source code for executables and run their testing without knowledge of how they work.
	DuatTestOptions = [][]string{ //nolint
		{""},
	}

	// The location that the annotations downward API mounted kubernetes files will be found
	k8sAnnotations = "/etc/podinfo/annotations" //nolint
)

func TestMain(m *testing.M) {
	// Allow the enclave for secrets to wipe things
	defer StopSecret()

	// Only perform this Parsed check inside the test framework. Do not be tempted
	// to do this in the main of our production package
	//
	if !flag.Parsed() {
		envflag.Parse()
	}

	// Make sure that any test files can be found via a valid topDir argument on the CLI
	if stat, err := os.Stat(*topDir); os.IsNotExist(err) {
		fmt.Println(kv.Wrap(err).With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime()))
		os.Exit(-1)
	} else {
		if !stat.Mode().IsDir() {
			fmt.Println(kv.NewError("not a directory").With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime()))
			os.Exit(-1)
		}

	}
	if dir, err := filepath.Abs(*topDir); err != nil {
		fmt.Println((kv.Wrap(err).With("top-dir", *topDir).With("stack", stack.Trace().TrimRuntime())))
	} else {
		flag.Set("top-dir", dir)
	}
	m.Run()
}
