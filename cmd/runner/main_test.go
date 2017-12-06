package main

import (
	"flag"
	"os"
	"testing"

	"github.com/karlmutch/envflag"
)

func init() {
}

var (
	parsedFlags = false

	TestStopC = make(chan bool)
)

// TestMain is invoked by the GoLang entry point for the runtime of compiled GoLang
// programs when the compiled and linked image has been run using the 'go test'
// command
//
// This function will invoke the applications entry point to initiate the normal execution flow
// of the server with the tests remaining under the scheduling control of the
// GoLang test runtime. For more information please read https://golang.org/pkg/testing/
//
func TestMain(m *testing.M) {

	// Only perform this Parsed check inside the test framework. Do not be tempted
	// to do this in the main of our production package
	//
	if !flag.Parsed() {
		envflag.Parse()
	}
	parsedFlags = true

	quitC := make(chan struct{})
	doneC := make(chan struct{})

	resultCode := -1
	{
		// Start the server under test
		go func() {
			logger.Info("Starting Server")
			if errs := EntryPoint(quitC, doneC); len(errs) != 0 {
				for _, err := range errs {
					logger.Error(err.Error())
				}
				os.Exit(-1)
			}

			<-quitC

			// When using benchmarking in production mode, that is no tests running the
			// user can park the server on a single unit test that only completes when this
			// channel is close, which happens only when there is a quitC from the application
			// due to a CTRL-C key sequence or kill -n command
			//
			// If the test was not selected for by the tester then this will be essentially a
			// NOP
			//
			close(TestStopC)

		}()

		// Wait for the server to signal it is ready for work
		<-doneC

		resultCode = m.Run()
	}

	logger.Info("waiting for server down to complete")

	// Wait until the main server is shutdown
	<-quitC

	if resultCode != 0 {
		os.Exit(resultCode)
	}
}
