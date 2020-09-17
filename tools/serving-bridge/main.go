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
	"runtime"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/leaf-ai/studio-go-runner/pkg/server"

	"github.com/davecgh/go-spew/spew"

	"github.com/karlmutch/envflag"

	"github.com/jjeffery/kv" // MIT License
)

var (
	// TestMode will be set to true if the test flag is set during a build when the exe
	// runs
	TestMode = false

	// Spew contains the process wide configuration preferences for the structure dumping
	// package
	Spew *spew.ConfigState

	buildTime string
	gitHash   string

	logger = log.NewLogger("serving-bridge")

	cfgNamespace = flag.String("k8s-namespace", "default", "The namespace that is being used for our configuration")
	cfgConfigMap = flag.String("k8s-configmap", "serving-bridge", "The name of the Kubernetes ConfigMap where this servers configuration can be found")

	tempOpt  = flag.String("working-dir", setTemp(), "the local working directory being used for runner storage, defaults to env var %TMPDIR, or /tmp")
	debugOpt = flag.Bool("debug", false, "leave debugging artifacts in place, can take a large amount of disk space (intended for developers only)")

	cpuProfileOpt   = flag.String("cpu-profile", "", "write a cpu profile to file")
	cpuProfileTimer = flag.String("cpu-profile-duration", "60s", "sets a time limit for CPU profiling after which it will be stopped, the server will continue to run however")

	promRefreshOpt = flag.Duration("prom-refresh", time.Duration(15*time.Second), "the refresh timer for the exported prometheus metrics service")
	promAddrOpt    = flag.String("prom-address", ":9090", "the address for the prometheus http server provisioned within the running server")
)

// initCPUProfiler is used to start a profiler for the CPU
func initCPUProfiler(ctx context.Context) {
	if len(*cpuProfileOpt) == 0 {
		logger.Info("Profiling not enabled")
		return
	}
	output, errGo := filepath.Abs(*cpuProfileOpt)
	if errGo != nil {
		logger.Fatal(errGo.Error())
	}
	f, errGo := os.Create(output)
	if errGo != nil {
		logger.Fatal(errGo.Error())
	}
	logger.Info("profiling enabled", "output", output, "duration", *cpuProfileTimer)
	pprof.StartCPUProfile(f)

	go cpuProfiler(ctx)

}

func cpuProfiler(ctx context.Context) {
	defer func() {
		pprof.StopCPUProfile()
		logger.Info("Profiling stopped")
	}()
	if len(*cpuProfileTimer) != 0 {
		timeout, errGo := time.ParseDuration(*cpuProfileTimer)
		if errGo != nil {
			logger.Warn("invalid cpu-profile-duration value, profiling stopped", "error", errGo.Error())
			return
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		<-ctx.Done()
		cancel()

		return
	}
	<-ctx.Done()
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

type callInfo struct {
	packageName string
	fileName    string
	funcName    string
	line        int
}

func retrieveCallInfo() (info *callInfo) {
	info = &callInfo{}

	pc, file, line, _ := runtime.Caller(2)

	_, info.fileName = path.Split(file)
	info.line = line

	runes := []rune(runtime.FuncForPC(pc).Name())
	if len(runes) > 8192 {
		runes = runes[:8192]
	}
	parts := strings.Split(string(runes), ".")
	pl := len(parts)
	info.funcName = parts[pl-1]

	if parts[pl-2][0] == '(' {
		info.funcName = parts[pl-2] + "." + info.funcName
		info.packageName = strings.Join(parts[0:pl-2], ".")
	} else {
		info.packageName = strings.Join(parts[0:pl-1], ".")
	}

	return info
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

	// Allow the enclave for secrets to wipe things
	runner.StopSecret()

	quitC := make(chan struct{})
	defer close(quitC)

	// This is the one check that does not get tested when the server is under test
	//
	if _, err := runner.NewExclusive("serving-bridge", quitC); err != nil {
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

	doneC := make(chan struct{})
	quitCtx, cancel := context.WithCancel(context.Background())

	// Start the profiler as early as possible and only in production will there
	// be a command line option to do it
	go initCPUProfiler(quitCtx)

	if errs := EntryPoint(quitCtx, cancel, doneC); len(errs) != 0 {
		for _, err := range errs {
			logger.Error(err.Error())
		}
		os.Exit(-1)
	}

	// After starting the application message handling loops
	// wait until the system has shutdown
	//
	<-quitCtx.Done()

	// Allow the quitC to be sent across the server for a short period of time before exiting
	time.Sleep(5 * time.Second)
}

// watchReportingChannels will monitor channels for events etc that will be reported
// to the output of the server.  Typically these events will originate inside
// libraries within the sever implementation that dont use logging packages etc
func watchReportingChannels(quitCtx context.Context, cancel context.CancelFunc) (stopC chan os.Signal, errorC chan kv.Error, statusC chan []string) {
	// Setup a channel to allow a CTRL-C to terminate all processing.  When the CTRL-C
	// occurs we cancel the background msg pump processing queue mesages from
	// the queue specific implementations, and this will also cause the main thread
	// to unblock and return
	//
	stopC = make(chan os.Signal)
	errorC = make(chan kv.Error)
	statusC = make(chan []string)
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
		case <-quitCtx.Done():
			logger.Warn("quitCtx Seen")
			return
		case <-stopC:
			logger.Warn("CTRL-C Seen")
			cancel()
			return
		}
	}()
	return stopC, errorC, statusC
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
func EntryPoint(quitCtx context.Context, cancel context.CancelFunc, doneC chan struct{}) (errs []kv.Error) {

	defer close(doneC)

	logger.Trace(fmt.Sprintf("%#v", retrieveCallInfo()))

	// Start a go function that will monitor all of the error and status reporting channels
	// for events and report these events to the output of the proicess etc
	stopC, errorC, statusC := watchReportingChannels(quitCtx, cancel)

	signal.Notify(stopC, os.Interrupt, syscall.SIGTERM)

	// One of the first thimgs to do is to determine if ur configuration is
	// coming from a remote source which in our case will typically be a
	// k8s configmap that is not supplied by the k8s deployment spec.  This
	// happens when the config map is to be dynamically tracked to allow
	// the runner to change is behaviour or shutdown etc

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
	readyC := make(chan struct{})
	go server.InitiateK8s(quitCtx, *cfgNamespace, *cfgConfigMap, readyC, logger, errorC)
	<-readyC

	errs = validateServerOpts()

	// Now check for any fatal kv.before allowing the system to continue.  This allows
	// all kv.that could have ocuured as a result of incorrect options to be flushed
	// out rather than having a frustrating single failure at a time loop for users
	// to fix things
	//
	if len(errs) != 0 {
		return errs
	}

	// None blocking function that initializes independent services in the runner
	startServices(quitCtx, statusC, errorC)

	return nil
}

func startServices(quitCtx context.Context, statusC chan []string, errorC chan kv.Error) {

	// Non blocking function to initialize the exporter of task resource usage for
	// prometheus
	server.StartPrometheusExporter(quitCtx, *promAddrOpt, &server.Resources{}, *promRefreshOpt, logger)

	// The timing for queues being refreshed should me much more frequent when testing
	// is being done to allow short lived resources such as queues etc to be refreshed
	// between and within test cases reducing test times etc, but not so quick as to
	// hide or shadow any bugs or issues
	serviceIntervals := time.Duration(15 * time.Second)
	if TestMode {
		serviceIntervals = time.Duration(5 * time.Second)
	}

	// Create a component that listens to S3 for new or modified index files
	//
	go serviceIndexes(quitCtx, serviceIntervals)
}
