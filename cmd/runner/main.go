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
	buildTime string
	gitHash   string

	logger = log.New("runner")

	tempOpt    = flag.String("working-dir", setTemp(), "the local working directory being used for runner storage, defaults to env var %TMPDIR, or /tmp")
	debugOpt   = flag.Bool("debug", false, "leave debugging artifacts in place, can take a large amount of disk space (intended for developers only)")
	cpuOnlyOpt = flag.Bool("cpu-only", false, "in the event no gpus are found continue with only CPU support")

	maxCoresOpt = flag.Uint("max-cores", 0, "maximum number of cores to be used (default 0, all cores available will be used)")
	maxMemOpt   = flag.String("max-mem", "0gb", "maximum amount of memory to be allocated to tasks using SI, ICE units, for example 512gb, 16gib, 1024mb, 64mib etc' (default 0, is all available RAM)")
	maxDiskOpt  = flag.String("max-disk", "0gb", "maximum amount of local disk storage to be allocated to tasks using SI, ICE units, for example 512gb, 16gib, 1024mb, 64mib etc' (default 0, is 85% of available Disk)")
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
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      studioml runner      ", gitHash, "    ", buildTime)
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

func resourceLimits() (cores uint, mem uint64, storage uint64, err error) {
	cores = *maxCoresOpt
	if mem, err = humanize.ParseBytes(*maxMemOpt); err != nil {
		return 0, 0, 0, err
	}
	if storage, err = humanize.ParseBytes(*maxDiskOpt); err != nil {
		return 0, 0, 0, err
	}
	return cores, mem, storage, err
}

func main() {

	fmt.Printf("%s built at %s, against commit id %s\n", os.Args[0], buildTime, gitHash)

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	// First gather any and as many errors as we can before stopping to allow one pass at the user
	// fixing things than than having them retrying multiple times
	fatalErr := false

	if _, free := runner.GPUSlots(); free == 0 {
		fmt.Fprintln(os.Stderr, "no available GPUs could be detected using the nvidia management library")
		if !*cpuOnlyOpt {
			fatalErr = true
		}
	}

	if len(*tempOpt) == 0 {
		fmt.Fprintln(os.Stderr, "the working-dir command line option must be supplied with a valid working directory location, or the TEMP, or TMP env vars need to be set")
		fatalErr = true
	}

	if _, _, err := getCacheOptions(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		fatalErr = true
	}

	// Attempt to deal with user specified hard limits on the CPU, this is a validation step for options
	// from the CLI
	//
	limitCores, limitMem, limitDisk, err := resourceLimits()
	if err = runner.SetCPULimits(limitCores, limitMem); err != nil {
		fmt.Fprintf(os.Stderr, "the cores, or memory limits on command line option were flawed due to %s\n", err.Error())
		fatalErr = true
	}
	avail, err := runner.SetDiskLimits(*tempOpt, limitDisk)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the disk storage limits on command line option were flawed due to %s\n", err.Error())
		fatalErr = true
	} else {
		if 0 == avail {
			fmt.Fprintf(os.Stderr, "insufficent disk storage available %s\n", humanize.Bytes(avail))
			fatalErr = true
		} else {
			logger.Debug(fmt.Sprintf("%s available diskspace", humanize.Bytes(avail)))
		}
	}

	// Supplying the context allows the client to pubsub to cancel the
	// blocking receive inside the run
	ctx, cancel := context.WithCancel(context.Background())

	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing pubsub mesages from
	// google, and this will also cause the main thread to unblock and return
	//
	stopC := make(chan os.Signal)
	quitC := make(chan bool)
	go func() {
		defer cancel()
		defer close(quitC)

		select {
		case <-stopC:
			log.Warn("CTRL-C Seen")
			return
		}
	}()

	signal.Notify(stopC, os.Interrupt, syscall.SIGTERM)

	// initialize the disk based artifact cache, after the signal handlers are in place
	//
	if err = runObjCache(ctx); err != nil {
		logger.Error(fmt.Sprintf("disk cache could not start, %s", err))
		fatalErr = true
	}

	credFile := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if len(credFile) == 0 {
		fmt.Fprintln(os.Stderr, "The GOOGLE_APPLICATION_CREDENTIALS must be set for the runner to work")
		fatalErr = true
	}

	// Get the default credentials to determine the default project ID
	//
	cred, err := google.FindDefaultCredentials(context.Background(), "")
	if err != nil {
		fmt.Fprintln(os.Stderr, "The google credentials file", credFile, "could not be processed due to", err.Error(), "please set the GOOGLE_APPLICATION_CREDENTIALS to a valid credentials file name")
		fatalErr = true
	}

	// Now check for any fatal errors before allowing the system to continue.  This allows
	// all errors that could have ocuured as a result of incorrect options to be flushed
	// out rather than having a frustrating single failure at a time loop for users
	// to fix things
	//
	if fatalErr {
		os.Exit(-1)
	}
	projectId := cred.ProjectID

	// Place useful messages into the slack monitoring channel if available
	host := runner.GetHostName()

	msg := fmt.Sprintf("started project %s on %s", projectId, host)
	logger.Info(msg)

	runner.InfoSlack(msg)
	defer runner.WarningSlack(fmt.Sprintf("stopping project %s on %s", projectId, host))

	// loops printing out resource consumption statistics on a regular basis
	go showResources(ctx)

	// start the prometheus http server for metrics
	go runPrometheus(ctx)

	// Now start processing the queues that exist within the project in the background
	qr, err := NewQueuer(projectId)
	if err != nil {
		log.Fatal(err.Error())
	}

	// Blocking until the server stops running the studioml queues, or the stop channel signals a shutdown attempt
	qr.run(quitC)

	// Allow the quitC to be sent across the server for a short period of time before exiting
	time.Sleep(time.Second)
}
