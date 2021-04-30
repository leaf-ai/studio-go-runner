// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-stack/stack"
	"github.com/leaf-ai/studio-go-runner/internal/defense"

	"github.com/jjeffery/kv" // MIT License
	"github.com/karlmutch/envflag"
)

var (
	parsedFlags = false

	TestStopC = make(chan bool)

	TestRunMain string

	// cleanupDirs is a list of working directories that need to be expunged when the test is finally all over
	// within this package
	cleanupDirs = []string{}

	// InitError is used to track an failures occurring during static initialization
	InitError kv.Error

	// TestOptions are externally visible symbols that this package is asking the unit test suite to pickup and use
	// when the testing is managed by an external entity, this allows build level variations that include or
	// exclude GPUs for example to run their tests appropriately.  It also allows the top level build logic
	// to inspect source code for executables and run their testing without knowledge of how they work.
	DuatTestOptions = [][]string{}

	topDir = flag.String("top-dir", "../..", "The location of the top level source directory for locating test files")
)

// When the runner tests are done we need to build the scenarios we want tested
// and their command line options for each case
func init() {
	cleanupDirs = append(cleanupDirs, "/tmp/cache-runner")
}

func cleanup() {
	for _, tmpDir := range cleanupDirs {
		os.RemoveAll(tmpDir)
	}

	// Allow the enclave for secrets to wipe things
	defense.StopSecret()
}

// TestRunMain can be used to run the command in production mode as opposed to
// funit or unit testing mode.  Traditionally gathering coverage data and running
// in production are done separately.  This unit test allows the runner to do
// both at the same time.  To do this a test binary is generated using the command
//
// cd $(GOROOT)/src/github.com/leaf-ai/studio-go-runner
// go test -coverpkg="." -c -o bin/queue-status tools/queue-status/*.go
//
// Then the resulting /bin/queue-status-coverage binary is run as though it were a traditional
// server binary for the daemon using the command below.  The difference being that the
// binary now has coverage instrumentation.  In order to collect the coverage run any production
// workload and use cases you need then CTRL-C the server.
//
// ./bin/queue-status-coverage -test.run "^TestRunMain$" -test.coverprofile=system.out
//
// As an additional feature coverage files have is that they can also be merged using
// commands similar to the following:
//
// $ go get github.com/wadey/gocovmerge
// $ gocovmerge unit.out system.out > all.out
// $ go tool cover -html all.out
//
// Using the coverage merge tool testing done using a fully deployed system with
// real projects, proxies, projects, and workloads along with integration testing can be merged
// together from different test steps in an integration and test pipeline.
//

// TestMain is invoked by the GoLang entry point for the runtime of compiled GoLang
// programs when the compiled and linked image has been run using the 'go test'
// command
//
// This function will invoke the applications entry point to initiate the normal execution flow
// of the server with the tests remaining under the scheduling control of the
// GoLang test runtime. For more information please read https://golang.org/pkg/testing/
//
func TestMain(m *testing.M) {

	defer cleanup()

	TestMode = true

	if InitError != nil {
		fmt.Fprintln(os.Stderr, InitError)
	}
	// Only perform this Parsed check inside the test framework. Do not be tempted
	// to do this in the main of our production package
	//
	if !flag.Parsed() {
		envflag.Parse()
	}
	parsedFlags = true

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

	os.Exit(m.Run())
}
