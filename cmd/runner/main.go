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

	"github.com/karlmutch/studio-go-runner"

	"github.com/karlmutch/envflag"
	"github.com/mgutz/logxi/v1"
)

var (
	logger log.Logger

	projectOpt = flag.String("tf-project", "tfstudio-a8367", "the google project id")
	queueOpt   = flag.String("tf-queue", "TFStudioTest", "the google project id")
)

func init() {
	logger = log.New("runner")
}

func usage() {
	fmt.Fprintln(os.Stderr, path.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      Run the TFStudio, DarkCycle® gateway")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "The GOOGLE_APPLICATION_CREDENTIALS env variable needs to be set before running this software.")
	fmt.Fprintln(os.Stderr, "More information can be found at https://developers.google.com/identity/protocols/application-default-credentials.")
	fmt.Fprintln(os.Stderr, "These credentials are used to access resources used by the TFStudio client to")
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

	// Supplying the context allows the client to pubsub to cancel the
	// blocking receive inside the run
	ctx, cancel := context.WithCancel(context.Background())

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

	newCtx, newCancel := context.WithTimeout(context.Background(), 5*time.Second)
	ps, err := runner.NewPubSub(newCtx, *projectOpt, *queueOpt, *queueOpt+"_sub")
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
				logger.Fatal(fmt.Sprintf("TFStudio message receiver stopped due to %s", err))
			case msg := <-ps.MsgC:
				rqst, err := runner.UnmarshalRequest(msg.Data)
				if err != nil {
					logger.Warn(fmt.Sprintf("could not unmarshal a msg from TFStudio due to %v", err))
					continue
				}
				logger.Info(fmt.Sprintf("%#v", rqst))
				msg.Ack()
			}
		}
	}()

	if err = ps.Start(ctx); err != nil {
		logger.Fatal(fmt.Sprintf("could not continue listening for msgs due to %v", err))
	}

	<-stopC
}
