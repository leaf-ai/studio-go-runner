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
)

var (
	logger = log.New("runner")

	queueOpt = flag.String("tf-queue", "", "the google project PubSub queue id")
)

func init() {
}

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

func main() {

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	if len(*queueOpt) == 0 {
		fmt.Fprintln(os.Stderr, "the tf-queue command line option must be supplied with a valid accessible Google PubSub queue")
		os.Exit(-1)
	}

	// Supplying the context allows the client to pubsub to cancel the
	// blocking receive inside the run
	ctx, cancel := context.WithCancel(context.Background())

	// Get the default credentials to determine the default project ID
	cred, err := google.FindDefaultCredentials(context.Background(), "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "The google credentials could not be found please set the GOOGLE_APPLICATION_CREDENTIALS to a valid credentials file name")
		os.Exit(-1)
	}
	projectId := cred.ProjectID

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
	ps, err := runner.NewPubSub(newCtx, projectId, *queueOpt, *queueOpt+"_sub")
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
				if err := processor.ProcessMsg(msg); err != nil {
					logger.Warn(fmt.Sprintf("could not process a msg from studioml due to %v", err))
				}
			}
		}
	}()

	if err = ps.Start(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("could not continue listening for msgs due to %v", err))
	}

	<-stopC
}
