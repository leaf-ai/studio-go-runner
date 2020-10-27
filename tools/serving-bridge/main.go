// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/leaf-ai/studio-go-runner/pkg/process"
	"github.com/leaf-ai/studio-go-runner/pkg/runtime"
	"github.com/leaf-ai/studio-go-runner/pkg/server"

	"github.com/karlmutch/envflag"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	// TestMode will be set to true if the test flag is set during a build when the exe
	// runs
	TestMode = false

	// Spew contains the process wide configuration preferences for the structure dumping
	// package
	Spew      *spew.ConfigState
	SpewSmall *spew.ConfigState // This variant

	buildTime string
	gitHash   string

	logger = log.NewLogger("serving-bridge")

	cfgNamespace = flag.String("k8s-namespace", "default", "The namespace that is being used for our configuration")
	cfgConfigMap = flag.String("k8s-configmap", "bridge-env", "The name of the Kubernetes ConfigMap where this servers up/down state configuration can be found")
	cfgHostName  = flag.String("k8s-node-name", "", "The host name name of the Kubernetes node where is server pod is deployed to")

	cfgMount = flag.String("k8s-cfg-params-mount", "", "The directory into which the servers parameters have been mounted for dynamic configuration")

	tempOpt  = flag.String("working-dir", setTemp(), "the local working directory being used for server storage, defaults to env var %TMPDIR, or /tmp")
	debugOpt = flag.Bool("debug", false, "leave debugging artifacts in place, can take a large amount of disk space (intended for developers only)")

	promRefreshOpt = flag.Duration("prom-refresh", time.Duration(15*time.Second), "the refresh timer for the exported prometheus metrics service")
	promAddrOpt    = flag.String("prom-address", ":9090", "the address for the prometheus http server provisioned within the running server")

	cpuProfileOpt = flag.String("cpu-profile", "", "write a cpu profile to file")

	serviceNameOpt = flag.String("service-name", "studio.ml/model-serving-bridge", "The logical service name for this service instance")

	o11yKey     = flag.String("o11y-key", "", "Honeycomb API key for OpenTelemetry exporter")
	o11yDataset = flag.String("o11y-dataset", "", "Honeycomb Dataset name for OpenTelemetry exporter")

	s3RefreshOpt = flag.Duration("s3-refresh", time.Duration(15*time.Second), "the refresh timer to check for index blob changes on s3/minio ")
)

func init() {
	Spew = spew.NewDefaultConfig()

	Spew.Indent = "    "
	Spew.SortKeys = true

	SpewSmall = spew.NewDefaultConfig()
	SpewSmall.Indent = " "
	SpewSmall.SortKeys = true
	SpewSmall.DisablePointerAddresses = true
	SpewSmall.DisableCapacities = true

	if TestMode {
		// When using test mode set the default referesh for s3 to be one third of the default refresh interval in production
		// to help speed testing along
		*s3RefreshOpt = minimumScanRate
	}
}

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
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      TFX export to serving bridge      ", gitHash, "    ", buildTime)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "options can be read for environment variables by changing dashes '-' to underscores")
	fmt.Fprintln(os.Stderr, "and using upper case letters.  The certs-dir option is a mandatory option.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "To control log levels the LOGXI env variables can be used, these are documented at https://github.com/mgutz/logxi")
}

// Go runtime entry point for production builds.  This function acts as an alias
// for the main.Main function.  This allows testing and code coverage features of
// go to invoke the logic within the server main without skipping important
// runtime initialization steps.  The coverage tools can then run this server as if it
// was a production binary.
//
// main will be called by the go runtime when the server is run in production mode
// avoiding this alias.
//
func main() {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This is the one check that does not get tested when the server is under test
	//
	if _, err := process.NewExclusive(ctx, "serving-bridge"); err != nil {
		logger.Error(fmt.Sprintf("An instance of this process is already running %s", err.Error()))
		os.Exit(-1)
	}

	Main()
}

// Main is a production style main that will invoke the server as a go routine to allow
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

	fmt.Printf("%s built at %s, against commit id %s\n", os.Args[0], buildTime, gitHash)

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Start the profiler as early as possible and only in production will there
		// be a command line option to do it
		if len(*cpuProfileOpt) != 0 {
			if err := runtime.InitCPUProfiler(ctx, *cpuProfileOpt); err != nil {
				logger.Error(err.Error())
			}
		}

		if errs := EntryPoint(ctx, nil); len(errs) != 0 {
			for _, err := range errs {
				logger.Error(err.Error())
			}
			os.Exit(-1)
		}
	}()

	// Allow the quitC to be sent across the server for a short period of time before exiting
	time.Sleep(5 * time.Second)
}

// watchReportingChannels will monitor channels for events etc that will be reported
// to the output of the server.  Typically these events will originate inside
// libraries within the sever implementation that dont use logging packages etc
func watchReportingChannels(terminateC chan struct{}) (errorC chan kv.Error, statusC chan []string) {
	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing queue mesages from
	// the queue specific implementations, and this will also cause the main thread
	// to unblock and return
	//
	stopC := make(chan os.Signal, 1)

	errorC = make(chan kv.Error)
	statusC = make(chan []string)
	go func() {
		defer close(stopC)
		defer func() {
			defer func() {
				recover()
			}()
			close(terminateC)
		}()

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
			case <-terminateC:
				logger.Warn("terminateC seen")
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

func validateServerOpts() (errs []kv.Error) {
	errs = []kv.Error{}

	if len(*tempOpt) == 0 {
		msg := "the working-dir command line option must be supplied with a valid working directory location, or the TEMP, or TMP env vars need to be set"
		errs = append(errs, kv.NewError(msg))
	}

	return errs
}

// EntryPoint enables both test and standard production infrastructure to
// invoke this server.
//
// quitC can be used by the invoking functions to stop the processing
// inside the server and exit from the EntryPoint function
//
// doneC is used by the EntryPoint function to indicate when it has terminated
// its processing
//
func EntryPoint(ctx context.Context, readyC chan *Listeners) (errs []kv.Error) {

	//ctx, cancel := context.WithCancel(ctx)
	//defer cancel()

	// Start a go function that will monitor all of the error and status reporting channels
	// for events and report these events to the output of the process etc
	terminateC := make(chan struct{}, 1)
	defer func() {
		defer func() {
			_ = recover()
		}()
		close(terminateC)
	}()
	errorC, statusC := watchReportingChannels(terminateC)

	// One of the first thimgs to do is to determine if ur configuration is
	// coming from a remote source which in our case will typically be a
	// k8s configmap that is not supplied by the k8s deployment spec.  This
	// happens when the config map is to be dynamically tracked to allow
	// the server to change is behaviour or shutdown etc

	logger.Info("version", "git_hash", gitHash)

	// Before continuing convert several if the directories specified in the CLI
	// to using absolute paths
	tmp, errGo := filepath.Abs(*tempOpt)
	if errGo == nil {
		*tempOpt = tmp
	}

	// Runs in the background handling the Kubernetes client subscription
	// that is used to monitor for configuration map based changes.  Wait
	// for its setup processing to be done before continuing
	k8sReadyC := make(chan struct{})
	go server.InitiateK8s(ctx, *cfgNamespace, *cfgConfigMap, k8sReadyC, logger, errorC)
	select {
	case <-k8sReadyC:
	case <-ctx.Done():
		return []kv.Error{kv.NewError("Termination before server initialized").With("stack", stack.Trace().TrimRuntime())}
	}

	errs = validateServerOpts()

	// Now check for any fatal kv.before allowing the system to continue.  This allows
	// all kv.that could have ocuured as a result of incorrect options to be flushed
	// out rather than having a frustrating single failure at a time loop for users
	// to fix things
	//
	if len(errs) != 0 {
		return errs
	}

	cfgUpdater := startDynamicCfg(ctx, *cfgMount, errorC)

	// Non-blocking function that initializes independent services in the server
	startServices(ctx, *serviceNameOpt, cfgUpdater, statusC, errorC)

	// Let others know when the startup function has dispatched all of the services
	// useful for testing
	if readyC != nil {
		func() {
			defer func() {
				recover()
			}()
			readyC <- cfgUpdater
		}()
	}

	logger.Info("server components initiated")

	defer func() {
		recover()
	}()
	<-ctx.Done()

	return nil
}

func startDynamicCfg(ctx context.Context, cfgMount string, errorC chan kv.Error) (cfgUpdater *Listeners) {
	// Initialize a starting cfg using the non dynamic CLI and environment variables
	// and let the configuration update handle the rest
	startingCfg, err := GetDefaultCfg()
	if err != nil {
		select {
		case errorC <- err:
		case <-time.After(time.Second):
		}
	}

	// Configuration dynamic update facility
	if cfgUpdater = NewConfigBroadcast(ctx, *startingCfg, errorC); cfgUpdater == nil {
		return cfgUpdater
	}

	// Now we start the dynamic ConfigMap based watcher that can be used to broadcast configuration
	// changes to components within this server
	if len(cfgMount) != 0 {
		go startCfgUpdater(ctx, cfgUpdater, cfgMount, errorC)
	}
	return cfgUpdater
}

func startServices(ctx context.Context, serviceName string, cfgUpdater *Listeners, statusC chan []string, errorC chan kv.Error) {

	// Non blocking function to initialize the exporter of task resource usage for
	// prometheus
	server.StartPrometheusExporter(ctx, *promAddrOpt, &server.Resources{}, *promRefreshOpt, logger)

	// Start the Go beeline telemetry for Honeycomb
	ctx, err := server.StartTelemetry(ctx, logger, *cfgHostName, serviceName, *o11yKey, *o11yDataset)
	if err != nil {
		logger.Warn(err.Error())
	}

	// The timing for queues being refreshed should me much more frequent when testing
	// is being done to allow short lived resources such as queues etc to be refreshed
	// between and within test cases reducing test times etc, but not so quick as to
	// hide or shadow any bugs or issues
	serviceIntervals := *s3RefreshOpt

	// Setup the retries policies for communicating with the S3 service endpoint
	backoffs := backoff.NewExponentialBackOff()
	backoffs.InitialInterval = serviceIntervals
	backoffs.Multiplier = 1.5
	backoffs.MaxElapsedTime = serviceIntervals * 5
	backoffs.Stop = serviceIntervals * 4

	// Create a component that listens to S3 for new or modified index files
	//
	go serviceIndexes(ctx, cfgUpdater, backoffs, logger)

	// Create the component that will scrape and update a TFX based model server configuration
	// file based on the in memory indexes that have been loaded from S3
	go tfxConfig(ctx, cfgUpdater, backoffs, logger)
}
