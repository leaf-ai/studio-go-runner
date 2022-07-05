// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/karlmutch/go-shortid"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/andreidenissov-cog/go-service/pkg/aws_gsc"
	"github.com/andreidenissov-cog/go-service/pkg/log"
	"github.com/andreidenissov-cog/go-service/pkg/process"
	"github.com/andreidenissov-cog/go-service/pkg/runtime"
	"github.com/andreidenissov-cog/go-service/pkg/server"

	"github.com/leaf-ai/studio-go-runner/internal/cpu_resource"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"
	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"github.com/leaf-ai/studio-go-runner/internal/disk_resource"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/davecgh/go-spew/spew"

	"github.com/karlmutch/envflag"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/dustin/go-humanize"
)

var (
	// TestMode will be set to true if the test flag is set during a build when the exe
	// runs
	TestMode = false

	// Spew contains the process wide configuration preferences for the structure dumping
	// package
	Spew *spew.ConfigState

	logger = log.NewLogger("runner")

	cfgNamespace = flag.String("k8s-namespace", "default", "The namespace that is being used for our configuration")
	cfgConfigMap = flag.String("k8s-configmap", "studioml-go-runner", "The name of the Kubernetes ConfigMap where our configuration can be found")

	amqpURL    = flag.String("amqp-url", "", "The URL for an amqp message exchange through which StudioML is being sent work")
	amqpMgtURL = flag.String("amqp-mgt-url", "", "The URL for the management interface for an amqp message exchange which StudioML can use to query the broker for queue stats etc")

	tempOpt    = flag.String("working-dir", setTemp(), "the local working directory being used for runner storage, defaults to env var %TMPDIR, or /tmp")
	debugOpt   = flag.Bool("debug", false, "leave debugging artifacts in place, can take a large amount of disk space (intended for developers only)")
	cpuOnlyOpt = flag.Bool("cpu-only", false, "in the event no gpus are found continue with only CPU support")

	maxCoresOpt = flag.Uint("max-cores", 0, "maximum number of cores to be used (default 0, all cores available will be used)")
	maxMemOpt   = flag.String("max-mem", "0gb", "maximum amount of memory to be allocated to tasks using SI, ICE units, for example 512gb, 16gib, 1024mb, 64mib etc' (default 0, is all available RAM)")
	maxDiskOpt  = flag.String("max-disk", "0gb", "maximum amount of local disk storage to be allocated to tasks using SI, ICE units, for example 512gb, 16gib, 1024mb, 64mib etc' (default 0, is 85% of available Disk)")

	msgEncryptDirOpt   = flag.String("encrypt-dir", "./certs/message", "directory where secrets have been mounted into pod containers")
	acceptClearTextOpt = flag.Bool("clear-text-messages", false, "enables clear-text messages across queues support (Associated Risk)")

	cpuProfileOpt = flag.String("cpu-profile", "", "write a cpu profile to file")

	sigsRqstDirOpt = flag.String("request-signatures-dir", "./certs/queues/signing", "the directory for queue message signing files")

	// rqstSigs contains a map with the index being the prefix of queue names and their public keys for inbound request queues
	rqstSigs = &defense.PubkeyStore{}

	sigsRspnsDirOpt = flag.String("response-signatures-dir", "./certs/queues/response-encrypt", "the directory for response queue message encryption files")

	// rqstSigs contains a map with the index being the prefix of queue names and their public keys for inbound request queues
	rspnsEncrypt = &defense.PubkeyStore{}

	promAddrOpt = flag.String("prom-address", ":9090", "the address for the prometheus http server within the runner")

	captureOutputMD = flag.Bool("schema-logs", true, "automatically add experiment logs to metadata json")

	generateMetaData = flag.Bool("generate-metadata", false, "generate experiment meta-data")

	localQueueRootOpt = flag.String("queue-root", "", "Local file path to directory serving as a root for local file queues")
)

// GetRqstSigs returns the signing public key struct for
// methods related to signature selection etc.
//
func GetRqstSigs() (s *defense.PubkeyStore) {
	return rqstSigs
}

// GetRspnsSigs returns the encryption public key struct for
// methods related to signature selection etc.
//
func GetRspnsEncrypt() (s *defense.PubkeyStore) {
	return rspnsEncrypt
}

func init() {
	Spew = spew.NewDefaultConfig()

	Spew.Indent = "    "
	Spew.SortKeys = true
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
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[arguments]      studioml runner      ", gitCommit, "    ", gitBranch)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "runner options can be read for environment variables by changing dashes '-' to underscores")
	fmt.Fprintln(os.Stderr, "and using upper case letters.  The certs-dir option is a mandatory option.")
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

// Go runtime entry point for production builds.  This function acts as an alias
// for the main.Main function.  This allows testing and code coverage features of
// go to invoke the logic within the server main without skipping important
// runtime initialization steps.  The coverage tools can then run this server as if it
// was a production binary.
//
// main will be called by the go runtime when the master is run in production mode
// avoiding this alias.
//
func main() {

	// Allow the enclave for secrets to wipe things
	defense.StopSecret()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// This is the one check that does not get tested when the server is under test
	//
	if _, err := process.NewExclusive(ctx, "studio-go-runner"); err != nil {
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

	fmt.Printf("%s built from branch %s, against commit id %s\n", os.Args[0], gitBranch, gitCommit)

	flag.Usage = usage

	// Use the go options parser to load command line options that have been set, and look
	// for these options inside the env variable table
	//
	envflag.Parse()

	doneC := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())

	// Start the profiler as early as possible and only in production will there
	// be a command line option to do it
	if err := runtime.InitCPUProfiler(ctx, *cpuProfileOpt); err != nil {
		logger.Warn(err.Error())
	}

	if errs := EntryPoint(ctx, cancel, doneC); len(errs) != 0 {
		for _, err := range errs {
			logger.Error(err.Error())
		}
		os.Exit(-1)
	}

	// After starting the application message handling loops
	// wait until the system has shutdown
	//
	<-ctx.Done()

	// Allow the quitC to be sent across the server for a short period of time before exiting
	time.Sleep(5 * time.Second)
}

func showAllStackTraces() {
	// Create a file for our debug info
	sid, errGo := shortid.Generate()
	if errGo != nil {
		sid = "xxx"
	}
	fn := filepath.Join(".", "stack-traces-"+sid+".txt")
	f, errGo := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		err := kv.Wrap(errGo).With("file", fn).With("stack", stack.Trace().TrimRuntime())
		fmt.Printf("FAILED to create debug info file %s\n", err.Error())
		return
	}
	defer f.Close()
	pprof.Lookup("goroutine").WriteTo(f, 1)
}

// watchDebugChannel will monitor internally created channel
// for external user-level signal to trigger some debugging actions.
func watchDebugChannel(ctx context.Context) {
	debugTrigger := make(chan os.Signal, 2)
	signal.Notify(debugTrigger, syscall.SIGUSR1, syscall.SIGUSR2)

	go func() {
		select {
		case <-ctx.Done():
			logger.Warn("watchDebugChannel: quit ctx Seen")
			return
		}
	}()
	go func() {
		for {
			select {
			case <-debugTrigger:
				logger.Warn("watchDebugChannel: debug action triggered")
				time.Sleep(1 * time.Second)
				showAllStackTraces()
			}
		}
	}()
}

// watchReportingChannels will monitor channels for events etc that will be reported
// to the output of the server.  Typically these events will originate inside
// libraries within the sever implementation that dont use logging packages etc
func watchReportingChannels(ctx context.Context, cancel context.CancelFunc) (stopC chan os.Signal, errorC chan kv.Error, statusC chan []string) {
	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing queue mesages from
	// the queue specific implementations, and this will also cause the main thread
	// to unblock and return
	//
	stopC = make(chan os.Signal, 2)
	errorC = make(chan kv.Error, 1)
	statusC = make(chan []string, 1)
	go func() {
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
			logger.Warn("quit ctx Seen")
			return
		}
	}()
	go func() {
		select {
		case <-stopC:
			logger.Warn("CTRL-C Seen")
			time.Sleep(1 * time.Second)
			cancel()
			return
		}
	}()
	return stopC, errorC, statusC
}

func validateGPUOpts() (errs []kv.Error) {
	errs = []kv.Error{}
	if !*cpuOnlyOpt {
		if _, free := cuda.GPUSlots(); free == 0 {
			if cuda.HasCUDA() {

				msg := fmt.Errorf("no available GPUs could be found using the nvidia management library")
				if cuda.CudaInitErr != nil {
					msg = *cuda.CudaInitErr
				}
				err := kv.Wrap(msg).With("stack", stack.Trace().TrimRuntime())
				if *debugOpt {
					logger.Warn(fmt.Sprint(err))
				} else {
					errs = append(errs, err)
				}
			}
		}
	}
	return errs
}

func validateCredsOpts() (errs []kv.Error) {
	errs = []kv.Error{}

	// Make at least one of the credentials directories is valid, as long as this is not a test
	if TestMode {
		logger.Warn("running in test mode, queue validation not performed")
	} else {
		if len(*sqsCertsDirOpt) == 0 && len(*amqpURL) == 0 &&
			len(*localQueueRootOpt) == 0 {
			errs = append(errs, kv.NewError("One of the amqp-url, sqs-certs or queue-root options must be set for the runner to work"))
		} else {
			stat, err := os.Stat(*sqsCertsDirOpt)
			if err != nil || !stat.Mode().IsDir() {
				if len(*amqpURL) == 0 {
					*localQueueRootOpt = os.ExpandEnv(*localQueueRootOpt)
					stat, err = os.Stat(*localQueueRootOpt)
					if err != nil || !stat.Mode().IsDir() {
						msg := fmt.Sprintf(
							"sqs-certs must be set to an existing directory, or amqp-url is specified, or queue-root must be set to an existing directory for the runner to perform any useful work (%s)",
							*sqsCertsDirOpt)
						errs = append(errs, kv.NewError(msg))
					}
				}
			}
		}
	}
	return errs
}

func validateResourceOpts() (errs []kv.Error) {
	errs = []kv.Error{}
	// Attempt to deal with user specified hard limits on the CPU, this is a validation step for options
	// from the CLI
	//
	limitCores, limitMem, limitDisk, err := resourceLimits()
	if err != nil {
		errs = append(errs, kv.Wrap(err).With("stack", stack.Trace().TrimRuntime()))
	}

	if err = cpu_resource.SetCPULimits(limitCores, limitMem); err != nil {
		errs = append(errs, kv.Wrap(err, "the cores, or memory limits on command line option were invalid").With("stack", stack.Trace().TrimRuntime()))
	}
	avail, err := disk_resource.SetDiskLimits(*tempOpt, limitDisk)
	if err != nil {
		errs = append(errs, kv.Wrap(err, "the disk storage limits on command line option were invalid").With("stack", stack.Trace().TrimRuntime()))
	} else {
		if 0 == avail {
			msg := fmt.Sprintf("insufficient disk storage available %s", humanize.Bytes(avail))
			errs = append(errs, kv.NewError(msg))
		} else {
			logger.Debug(fmt.Sprintf("%s available diskspace", humanize.Bytes(avail)))
		}
	}
	return errs
}

func validateServerOpts() (errs []kv.Error) {
	errs = []kv.Error{}

	// First gather any and as many kv.as we can before stopping to allow one pass at the user
	// fixing things than than having them retrying multiple times
	errs = append(errs, validateGPUOpts()...)

	if len(*tempOpt) == 0 {
		msg := "the working-dir command line option must be supplied with a valid working directory location, or the TEMP, or TMP env vars need to be set"
		errs = append(errs, kv.NewError(msg))
	}

	if _, _, err := getCacheOptions(); err != nil {
		errs = append(errs, kv.Wrap(err).With("stack", stack.Trace().TrimRuntime()))
	}

	errs = append(errs, validateResourceOpts()...)

	errs = append(errs, validateCredsOpts()...)

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
func EntryPoint(ctx context.Context, cancel context.CancelFunc, doneC chan struct{}) (errs []kv.Error) {

	defer close(doneC)

	// Start a go function that will monitor all of the error and status reporting channels
	// for events and report these events to the output of the process etc
	stopC, errorC, statusC := watchReportingChannels(ctx, cancel)

	signal.Notify(stopC, os.Interrupt, syscall.SIGTERM)

	watchDebugChannel(ctx)

	// One of the first thimgs to do is to determine if ur configuration is
	// coming from a remote source which in our case will typically be a
	// k8s configmap that is not supplied by the k8s deployment spec.  This
	// happens when the config map is to be dynamically tracked to allow
	// the runner to change is behaviour or shutdown etc

	logger.Info("version", "git_branch", gitBranch, "git_hash", gitCommit)

	if aws, err := aws_gsc.IsAWS(); aws {
		logger.Info("AWS detected")
	} else {
		if err == nil {
			logger.Info("AWS not detected")
		} else {
			logger.Info("AWS not detected", "error", err)
		}
	}

	// Before continuing convert several if the directories specified in the CLI
	// to using absolute paths
	tmp, errGo := filepath.Abs(*tempOpt)
	if errGo == nil {
		*tempOpt = tmp
	}
	tmp, errGo = filepath.Abs(*msgEncryptDirOpt)
	if errGo == nil {
		*msgEncryptDirOpt = tmp
	}
	tmp, errGo = filepath.Abs(*sigsRqstDirOpt)
	if errGo == nil {
		*sigsRqstDirOpt = tmp
	}
	tmp, errGo = filepath.Abs(*sigsRspnsDirOpt)
	if errGo == nil {
		*sigsRspnsDirOpt = tmp
	}

	// Runs in the background handling the Kubernetes client subscription
	// that is used to monitor for configuration map based changes.  Wait
	// for its setup processing to be done before continuing
	dedupeMsg := time.Duration(15 * time.Minute)
	readyC := make(chan struct{})
	go server.InitiateK8s(ctx, *cfgNamespace, *cfgConfigMap, readyC, dedupeMsg, logger, errorC)
	<-readyC

	errs = validateServerOpts()

	// initialize the disk based artifact cache, after the signal handlers are in place
	//
	if err := RunObjCache(ctx); err != nil {
		errs = append(errs, kv.Wrap(err))
	}

	if qerrs := runner.InitQueueMatcher(ctx, *cfgNamespace, *cfgConfigMap, logger); len(qerrs) > 0 {
		errs = append(errs, qerrs...)
	}

	// Now check for any fatal kv.before allowing the system to continue.  This allows
	// all kv.that could have ocuured as a result of incorrect options to be flushed
	// out rather than having a frustrating single failure at a time loop for users
	// to fix things
	//
	if len(errs) != 0 {
		return errs
	}

	// None blocking function that initializes independent services in the runner
	startServices(ctx, cancel, statusC, errorC)

	return nil
}

func startServices(ctx context.Context, cancel context.CancelFunc, statusC chan []string, errorC chan kv.Error) {
	// Watch for GPU hardware events that are of interest
	go cuda.MonitorGPUs(ctx, statusC, errorC)

	// loops doing prometheus exports for resource consumption statistics etc
	// on a regular basis
	//server.StartPrometheusExporter(ctx, *promAddrOpt, &resources.Resources{}, time.Duration(10*time.Second), logger)

	// The timing for queues being refreshed should me much more frequent when testing
	// is being done to allow short lived resources such as queues etc to be refreshed
	// between and within test cases reducing test times etc, but not so quick as to
	// hide or shadow any bugs or issues
	serviceIntervals := time.Duration(15 * time.Second)
	if TestMode {
		serviceIntervals = time.Duration(5 * time.Second)
	}

	// Setup a watcher that will scan a signatures directory loading in
	// new queue related message signing keys, non blocking function that
	// spins off a servicing function
	store, err := defense.InitRqstSigWatcher(ctx, *sigsRqstDirOpt, errorC)
	if err != nil {
		errorC <- err
	}
	rqstSigs = store

	// Setup a watcher that will scan a response encryption directory loading in
	// new response queue related message encryption keys, non blocking function that
	// spins off a servicing function
	if store, err = defense.InitRspnsEncryptWatcher(ctx, *sigsRspnsDirOpt, errorC); err != nil {
		errorC <- err
	}
	rspnsEncrypt = store

	// run a limiter that will check for various termination conditions for the
	// runner including idle times, and the maximum number of tasks to complete
	go serviceLimiter(ctx, cancel)

	// run a cleanup service for virtual environments cache:
	go runner.ServiceVirtualEnvCache(ctx)

	// Create a component that listens to AWS credentials directories
	// and starts and stops run methods as needed based on the credentials
	// it has for the AWS infrastructure
	//
	go serviceSQS(ctx, serviceIntervals)

	// Create a component that listens to an amqp (rabbitMQ) exchange for work
	// queues
	//
	go serviceRMQ(ctx, serviceIntervals, 15*time.Second)

	// Create a component that listens to local file queues root for work
	// queues
	//
	go serviceFileQueue(ctx, 3*time.Second)
}
