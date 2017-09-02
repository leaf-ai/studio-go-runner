package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"

	"github.com/karlmutch/envflag"
	"github.com/mgutz/logxi/v1"

	"golang.org/x/oauth2/google"

	"github.com/dustin/go-humanize"
)

var (
	logger = log.New("runner")

	queueOpt = flag.String("tf-queue", "", "the google project PubSub queue id")
	debugOpt = flag.Bool("debug", false, "leave debugging artifacts in place, can take a large amount of disk space (intended for developers only)")

	maxCoresOpt = flag.Uint("max-cores", 0, "maximum number of cores to be used (default 0, all cores available will be used)")
	maxMemOpt   = flag.String("max-mem", "0gb", "maximum amount of memory to be allocated to tasks using SI, ICE units, for example 512gb, 16gib, 1024mb, 64mib etc' (default 0, is all available RAM)")
)

func usage() {
	fmt.Fprintln(os.Stderr, path.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      Run the studioml, DarkCycle® gateway")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "The GOOGLE_APPLICATION_CREDENTIALS env variable needs to be set before running this software.")
	fmt.Fprintln(os.Stderr, "More information can be found at https://developers.google.com/identity/protocols/application-default-credentials.")
	fmt.Fprintln(os.Stderr, "These credentials are used to access resources used by the studioml client to")
	fmt.Fprintln(os.Stderr, "retrieve compute requests from users.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To control log levels the LOGXI env variables can be used, these are documented at https://github.com/mgutz/logxi")
}

func resourceLimits() (cores uint, mem uint64, err error) {
	cores = *maxCoresOpt
	mem, err = humanize.ParseBytes(*maxMemOpt)
	return cores, mem, err
}

func main() {

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	// First gather any and as many errors as we can before stopping to allow one pass at the user
	// fixing things than than having them retrying multiple times
	fatalErr := false

	if runner.GetGPUCount() == 0 {
		fmt.Fprintln(os.Stderr, "no GPUs could be detected using the nvidia management library")
		fatalErr = true
	}

	if len(*queueOpt) == 0 {
		fmt.Fprintln(os.Stderr, "the tf-queue command line option must be supplied with a valid accessible Google PubSub queue")
		fatalErr = true
	}

	// Attempt to deal with user specified hard limits on the CPU, this is a validation step for options
	// from the CLI
	//
	limitCores, limitMem, err := resourceLimits()
	if err = runner.SetCPULimits(limitCores, limitMem); err != nil {
		fmt.Fprintf(os.Stderr, "the cores, or memory limits on command line option were flawed due to %s\n", err.Error())
		fatalErr = true
	}

	// Supplying the context allows the client to pubsub to cancel the
	// blocking receive inside the run
	ctx, cancel := context.WithCancel(context.Background())

	// Get the default credentials to determine the default project ID
	cred, err := google.FindDefaultCredentials(context.Background(), "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "The google credentials could not be found please set the GOOGLE_APPLICATION_CREDENTIALS to a valid credentials file name")
		fatalErr = true
	}
	projectId := cred.ProjectID

	if fatalErr {
		os.Exit(-1)
	}

	// Post an informational message to get a timstamp in the log when running in INFO mode
	logger.Info(fmt.Sprintf("started using project %s", projectId))

	processor, err := newProcessor(projectId)
	if err != nil {
		logger.Fatal(fmt.Sprintf("firebase connection failed due to %v", err))
	}
	defer processor.Close()

	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing pubsub mesages from
	// google, and this will also cause the main thread to unblock and return
	//
	stopC := make(chan os.Signal)
	go func() {
		defer cancel()

		select {
		case <-stopC:
			log.Warn("CTRL-C Seen")
			return
		}
	}()

	signal.Notify(stopC, os.Interrupt, syscall.SIGTERM)

	newCtx, newCancel := context.WithTimeout(context.Background(), 10*time.Second)
	ps, err := runner.NewPubSub(newCtx, projectId, *queueOpt, *queueOpt)
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not start the pubsub listener due to %v", err))
	}
	newCancel()

	defer ps.Close()

	// Start an asynchronous function that will listen for messages from pubsub,
	// errors also being sent related to pubsub failures that might occur,
	// and also for cancellations being signalled via the processing context
	// this server will use
	//
	go func() {
		defer close(stopC)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ps.ErrorC:
				logger.Fatal(fmt.Sprintf("studioml message receiver stopped due to %s", err))
			case msg := <-ps.MsgC:
				// Currently blocking function for one job at a time but will convert to
				// async internally
				wait, err := processor.ProcessMsg(msg)
				if err != nil {
					logger.Warn(fmt.Sprintf("could not process a msg from studioml due to %v", err))
				}

				if wait != nil {
					// If we had an issue with allocation dont take more work until old work completes,
					// or for a backoff time period
					select {
					case <-time.After(*wait):
						continue
					case <-ctx.Done():
						return
						// The ready fires on a significant change that may or may not
						// allow new work to be handled
					case <-processor.ready:
						continue
					}
				}
			}
		}
	}()

	if err = ps.Start(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("could not continue listening for msgs due to %v", err))
	}

	<-stopC
}
