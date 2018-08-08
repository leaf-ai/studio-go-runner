package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	runner "github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/karlmutch/envflag"
	"github.com/karlmutch/errors"
	// MIT License
)

var (
	parsedFlags = false

	TestStopC = make(chan bool)

	TestRunMain string

	useGPU = flag.Bool("no-gpu", false, "Used to skip test and other initialization GPU hardware code")

	// cleanupDirs is a list of working directories that need to be expunged when the test is finally all over
	// within this package
	cleanupDirs = []string{}

	// InitError is used to track an failures occurring during static initialization
	InitError errors.Error

	// TestOptions are externally visible symbols that this package is asking the unit test suite to pickup and use
	// when the testing is managed by an external entity, this allows build level variations that include or
	// exclude GPUs for example to run their tests appropriately.  It also allows the top level build logic
	// to inspect source code for executables and run their testing without knowledge of how they work.
	DuatTestOptions = [][]string{
		{"-cache-dir=/tmp/cache-runner", "-cache-size=1Gib", "--cache-create"},
	}
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
}

// TestRunMain can be used to run the server in production mode as opposed to
// funit or unit testing mode.  Traditionally gathering coverage data and running
// in production are done separately.  This unit test allows the runner to do
// both at the same time.  To do this a test binary is generated using the command
//
// cd $(GOROOT)/src/github.com/SentientTechnologies/studio-go-runner
// go test -coverpkg="." -c -o bin/runner-cpu-run-coverage -tags 'NO_CUDA' cmd/runner/*.go
//
// Then the resulting /bin/runner-cpu-run-coverage binary is run as through it were a traditional
// server binary for the go runner using the command below.  The difference being that the
// binary now has coverage instrumentation.  In order to collect the coverage run any production
// workload and use cases you need then CTRL-C the server.
//
// ./bin/runner-cpu-run-coverage -test.run "^TestRunMain$" -test.coverprofile=system.out
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

	runner.UseGPU = useGPU

	quitCtx, quit := context.WithCancel(context.Background())
	initializedC := make(chan struct{})

	resultCode := -1
	{
		// Start the server under test
		go func() {
			logger.Info("starting server")
			if errs := EntryPoint(quitCtx, quit, initializedC); len(errs) != 0 {
				for _, err := range errs {
					logger.Error(err.Error())
				}
				logger.Fatal("test setup failed, aborting all testing")
				os.Exit(-2)
			}
			<-quitCtx.Done()
			// When using benchmarking in production mode, that is no tests running the
			// user can park the server on a single unit test that only completes when this
			// channel is close, which happens only when there is a quitCtx from the application
			// due to a CTRL-C key sequence or kill -n command
			//
			// If the test was not selected for by the tester then this will be essentially a
			// NOP
			//
			close(TestStopC)

			logger.Info("forcing test mode server down")
			func() {
				defer func() {
					recover()
				}()
				quit()
			}()

		}()

		// The initialization is done inline so that we know the test S3 server is
		// running prior to any testing starting
		logger.Info("starting interfaces such as minio (S3), and message queuing")
		errC := runner.LocalMinio(quitCtx)

		go func() {

			// Wait for any errors from the S3 server and log them, continuing until
			// the testing stops
			for {
				select {
				case err := <-errC:
					if err != nil {
						logger.Error(err.Error())
					}
				case <-quitCtx.Done():
					break
				}
			}
		}()

		// Wait for the server to signal it is ready for work
		<-initializedC

		// If there are any tests to be done we now start them
		if len(TestRunMain) != 0 {
			<-TestStopC
		} else {
			resultCode = m.Run()

			quit()
		}
	}

	logger.Info("waiting for server down to complete")

	// Wait until the main server is shutdown
	<-quitCtx.Done()

	time.Sleep(2 * time.Second)

	if resultCode != 0 {
		os.Exit(resultCode)
	}
}
