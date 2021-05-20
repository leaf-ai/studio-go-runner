// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/go-stack/stack"
	"github.com/leaf-ai/go-service/pkg/log"

	"github.com/karlmutch/envflag"

	"github.com/jjeffery/kv" // MIT License
)

var (
	// TestMode will be set to true if the test flag is set during a build when the exe
	// runs
	TestMode = false

	buildTime string
	gitHash   string

	logger = log.NewErrLogger("queue-status")

	debugOpt = flag.Bool("debug", false, "leave debugging artifacts in place, print internal execution information")
)

func setTemp() (dir string) {
	if dir = os.Getenv("TMPDIR"); len(dir) != 0 {
		return dir
	}
	if _, err := os.Stat("/tmp"); err == nil {
		dir = "/tmp"
	}
	return dir
}

func usage() {
	fmt.Fprintln(os.Stderr, path.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      SQS Queue Status tool      ", gitHash, "    ", buildTime)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "options can be read for environment variables by changing dashes '-' to underscores")
	fmt.Fprintln(os.Stderr, "and using upper case letters.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To control log levels the LOGXI env variables can be used, these are documented at https://github.com/mgutz/logxi")
	fmt.Fprintln(os.Stderr, "All logging output goes to stderr, stdout contains command output only.")
}

// Go runtime entry point for production builds.  This function acts as an alias
// for the main.Main function.  This allows testing and code coverage features of
// go to invoke the logic within the command main without skipping important
// runtime initialization steps.  The coverage tools can then run this server as if it
// was a production binary.
//
// main will be called by the go runtime when the server is run in production mode
// avoiding this alias.
//
func main() {

	Main()
}

// Main is a production style main that will invoke the command as a go routine to allow
// a very simple supervisor and a test wrapper to coexist in terms of our logic.
//
// When using test mode 'go test ...' this function will not, normally, be run and
// instead the EntryPoint function will be called avoiding some initialization
// logic that is not applicable when testing.  There is one exception to this
// and that is when the go unit test framework is linked to the master binary,
// using a TestRunMain build flag which allows a binary with coverage
// instrumentation to be compiled with only a single unit test which is,
// infact an alias to this main.
//
func Main() {

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if errs := EntryPoint(ctx, cancel); len(errs) != 0 {
		for _, err := range errs {
			logger.Error(err.Error())
		}
		os.Exit(-1)
	}
}

// watchReportingChannels will monitor channels for events etc that will be reported
// to the output of the command.  Typically these events will originate inside
// libraries within the command implementation that dont use logging packages etc
func watchReportingChannels(ctx context.Context, cancel context.CancelFunc) (errorC chan kv.Error, statusC chan []string) {
	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing queue mesages from
	// the queue specific implementations, and this will also cause the main thread
	// to unblock and return
	//
	stopC := make(chan os.Signal, 1)

	errorC = make(chan kv.Error)
	statusC = make(chan []string)

	go func() {
		defer cancel()
		for {
			select {
			case msgs := <-statusC:
				switch len(msgs) {
				case 0:
				case 1:
					logger.Info(msgs[0])
				default:
					logger.Info(msgs[0], msgs[1:])
				}
			case err := <-errorC:
				if err != nil {
					logger.Warn(fmt.Sprint(err))
				}
			case <-ctx.Done():
				return
			case <-stopC:
				logger.Warn("CTRL-C seen")
				return
			}
		}
	}()

	signal.Reset()
	signal.Notify(stopC, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	return errorC, statusC
}

// EntryPoint enables both test and standard production infrastructure to
// invoke this command.
//
func EntryPoint(ctx context.Context, cancel context.CancelFunc) (errs []kv.Error) {

	// Start a go function that will monitor all of the error and status reporting channels
	// for events and report these events to the output of the process etc
	_, _ = watchReportingChannels(ctx, cancel)

	cfg, err := GetDefaultCfg()
	if err != nil {
		return append(errs, err)
	}

	// Function to query queue lists
	queues, err := GetQueues(ctx, cfg)
	if err != nil {
		return []kv.Error{err}
	}
	// Function to display the results

	json, errGo := json.MarshalIndent(queues, "", "    ")
	if errGo != nil {
		return []kv.Error{kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())}
	}
	fmt.Println((string)(json))
	return nil
}
