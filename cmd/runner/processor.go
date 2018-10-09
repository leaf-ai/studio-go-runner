package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/dgryski/go-farm"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"

	"github.com/dustin/go-humanize"
	"github.com/karlmutch/go-shortid"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

type processor struct {
	Group      string            `json:"group"` // A caller specific grouping for work that can share sensitive resources
	RootDir    string            `json:"root_dir"`
	ExprDir    string            `json:"expr_dir"`
	ExprSubDir string            `json:"expr_sub_dir"`
	ExprEnvs   map[string]string `json:"expr_envs"`
	Request    *runner.Request   `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
	Creds      string            `json:"credentials_file"`
	Artifacts  *runner.ArtifactCache
	Executor   Executor
	ready      chan bool // Used by the processor to indicate it has released resources or state has changed
}

type tempSafe struct {
	dir string
	sync.Mutex
}

var (
	// This is a safety valve for when work should not be scheduled due to allocation
	// failing to get resources.  In these case we wait for another job to complete however
	// this might no occur for sometime and we might want to come back around and see
	// if a smaller job is available.  But we only do this after a backoff period to not
	// hammer queues relentlessly
	//
	errBackoff = time.Duration(5 * time.Minute)

	// Used to store machine resource prfile
	resources = &runner.Resources{}

	// tempRoot is used to store information about the root directory uses by the
	// runner
	tempRoot = tempSafe{}

	// A shared cache for all projects exists that is used by processors
	artifactCache = runner.NewArtifactCache()

	// Used to initialize a logger sink into which the artifact caching code in the runner
	// can send error messages for the application to determine what action is taken with
	// caching errors that might be short lived
	cacheReport sync.Once
)

func init() {
	res, err := runner.NewResources(*tempOpt)
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not initialize disk space tracking due to %s", err.Error()))
	}
	resources = res

	// A cache exists on linux for cuda lets remove it as it
	// can cause issues
	errGo := os.RemoveAll("$HOME/.nv")
	if errGo != nil {
		logger.Fatal(fmt.Sprintf("could not clear the $HOME/.nv cache due to %s", err.Error()))
	}
}

func cacheReporter(quitC <-chan struct{}) {
	for {
		select {
		case err := <-artifactCache.ErrorC:
			logger.Info(fmt.Sprintf("cache error %v", err))
		case <-quitC:
			return
		}
	}
}

// Executor is an interface that defines a job handling worker implementation.  Each variant of a worker
// conforms to a standard processor interface
//
type Executor interface {

	// Make is used to allow a script to be generated for the specific run strategy being used
	Make(alloc *runner.Allocated, e interface{}) (err errors.Error)

	// Run will execute the worker task used by the experiment
	Run(ctx context.Context, refresh map[string]runner.Artifact) (err errors.Error)

	// Close can be used to tidy up after an experiment has completed
	Close() (err errors.Error)
}

// newProcessor will create a new working directory
//
func newProcessor(group string, msg []byte, creds string, quitC <-chan struct{}) (proc *processor, err errors.Error) {

	// When a processor is initialized make sure that the logger is enabled first time through
	//
	cacheReport.Do(func() {
		go cacheReporter(quitC)
	})

	temp, err := func() (temp string, err errors.Error) {
		// Singleton style initialization to instantiate and overridding directory
		// for the entire server working area
		//
		tempRoot.Lock()
		defer tempRoot.Unlock()

		if tempRoot.dir == "" {
			id, errGo := shortid.Generate()
			if errGo != nil {
				return "", errors.Wrap(errGo, "temp file id generation failed").With("stack", stack.Trace().TrimRuntime())
			}
			if tempRoot.dir, errGo = ioutil.TempDir(*tempOpt, "gorun_"+id); errGo != nil {
				return "", errors.Wrap(errGo, "temp file create failed").With("stack", stack.Trace().TrimRuntime())
			}
		}
		return tempRoot.dir, nil

	}()
	if err != nil {
		return nil, err
	}

	// Processors share the same root directory and use acccession numbers on the experiment key
	// to avoid collisions
	//
	p := &processor{
		RootDir: temp,
		Group:   group,
		Creds:   creds,
		ready:   make(chan bool),
	}

	// restore the msg into the processing data structure from the JSON queue payload
	p.Request, err = runner.UnmarshalRequest(msg)
	if err != nil {
		return nil, err
	}

	if _, err = p.mkUniqDir(); err != nil {
		return nil, err
	}

	// Determine the type of execution that is needed for this job by
	// inspecting the artifacts specified
	//
	mode := ExecUnknown
	for group := range p.Request.Experiment.Artifacts {
		if len(group) == 0 {
			continue
		}
		switch group {
		case "workspace":
			if mode == ExecUnknown {
				mode = ExecPythonVEnv
			}
		case "_singularity":
			mode = ExecSingularity
			break
		}
	}

	switch mode {
	case ExecPythonVEnv:
		if p.Executor, err = runner.NewVirtualEnv(p.Request, p.ExprDir); err != nil {
			return nil, err
		}
	case ExecSingularity:
		if p.Executor, err = runner.NewSingularity(p.Request, p.ExprDir); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unable to determine execution class from artifacts").With("stack", stack.Trace().TrimRuntime()).
			With("project", p.Request.Config.Database.ProjectId).With("experiment", p.Request.Experiment.Key)
	}

	logger.Info("experiment dir '" + p.ExprDir + "' is being used")

	return p, nil
}

const (
	ExecUnknown     = iota // ExecUnknown is an unused guard value
	ExecPythonVEnv         // Using the python virtualenv packaging
	ExecSingularity        // Using the Singularity container packaging and runtime
)

// Close will release all resources and clean up the work directory that
// was used by the studioml work
//
func (p *processor) Close() (err error) {
	if *debugOpt || 0 == len(p.ExprDir) {
		logger.Info("experiment dir " + p.ExprDir + " has been preserved")
		return nil
	}

	logger.Debug("remove experiment dir " + p.ExprDir)
	return os.RemoveAll(p.ExprDir)
}

// fetchAll is used to retrieve from the storage system employed by studioml any and all available
// artifacts and to unpack them into the experiment directory
//
func (p *processor) fetchAll() (err errors.Error) {

	for group, artifact := range p.Request.Experiment.Artifacts {

		// Artifacts that have no qualified location will be ignored
		if 0 == len(artifact.Qualified) {
			continue
		}

		// This artifact is downloaded during the runtime pass not beforehand
		if group == "_singularity" {
			continue
		}

		// Extract all available artifacts into subdirectories of the main experiment directory.
		//
		// The current convention is that the archives include the directory name under which
		// the files are unpacked in their table of contents
		//
		if warns, err := artifactCache.Fetch(&artifact, p.Request.Config.Database.ProjectId, group, p.Creds, p.ExprEnvs, p.ExprDir); err != nil {
			msg := err.With("group", group).With("project", p.Request.Config.Database.ProjectId).With("Experiment", p.Request.Experiment.Key).Error()
			if artifact.Mutable {
				logger.Debug(msg)
			} else {
				logger.Warn(msg)
			}
			for _, warn := range warns {
				msg = warn.With("group", group).With("project", p.Request.Config.Database.ProjectId).With("Experiment", p.Request.Experiment.Key).Error()
				if artifact.Mutable {
					logger.Debug(msg)
				} else {
					logger.Warn(msg)
				}
			}

			// Mutable artifacts can be create only items that dont yet exist on the storage platform
			if !artifact.Mutable {
				return err
			}
		}
	}
	return nil
}

// returnOne is used to upload a single artifact to the data store specified by the experimenter
//
func (p *processor) returnOne(group string, artifact runner.Artifact) (uploaded bool, warns []errors.Error, err errors.Error) {

	uploaded, warns, err = artifactCache.Restore(&artifact, p.Request.Config.Database.ProjectId, group, p.Creds, p.ExprEnvs, p.ExprDir)
	if err != nil {
		runner.WarningSlack(p.Request.Config.Runner.SlackDest, fmt.Sprintf("output from %s %s %v could not be returned due to %s", p.Request.Config.Database.ProjectId,
			p.Request.Experiment.Key, artifact, err.Error()), []string{})
	}
	return uploaded, warns, err
}

// returnAll creates tar archives of the experiments artifacts and then puts them
// back to the studioml shared storage
//
func (p *processor) returnAll() (warns []errors.Error, err errors.Error) {

	returned := make([]string, 0, len(p.Request.Experiment.Artifacts))

	for group, artifact := range p.Request.Experiment.Artifacts {
		if artifact.Mutable {
			if _, warns, err = p.returnOne(group, artifact); err != nil {
				return warns, err
			}
		}
	}

	if len(returned) != 0 {
		logger.Info(fmt.Sprintf("project %s returning %s", p.Request.Config.Database.ProjectId, strings.Join(returned, ", ")))
	}

	return warns, nil
}

// slackOutput is used to send logging information to the slack channels used for
// observing the results and failures within experiments
//
func (p *processor) slackOutput() {
	_, isPresent := p.Request.Experiment.Artifacts["output"]
	if !isPresent {
		err := errors.New("output artifact not present when job terminated").With("stack", stack.Trace().TrimRuntime()).
			With("project", p.Request.Config.Database.ProjectId).With("experiment", p.Request.Experiment.Key)
		runner.WarningSlack(p.Request.Config.Runner.SlackDest, fmt.Sprintf("%v", err), []string{})
		logger.Warn(err.Error())
		return
	}

	fn, err := artifactCache.Local("output", p.ExprDir, "output")
	if err != nil {
		runner.WarningSlack(p.Request.Config.Runner.SlackDest, fmt.Sprintf("%v", err), []string{})
		logger.Warn(err.Error())
		return
	}

	chunkSize := uint32(7 * 1024) // Slack has a limit of 8K bytes on attachments, leave some spare for formatting etc

	data, err := runner.ReadLast(fn, chunkSize)
	if err != nil {
		runner.WarningSlack(p.Request.Config.Runner.SlackDest, err.Error(), []string{})
		logger.Warn(err.Error())
		return
	}

	runner.InfoSlack(p.Request.Config.Runner.SlackDest, fmt.Sprintf("output from %s %s", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key), []string{data})
}

// allocate is used to reserve the resources on the local host needed to handle the entire job as
// a highwater mark.
//
// The returned alloc structure should be used with the deallocate function otherwise resource
// leaks will occur.
//
func (p *processor) allocate() (alloc *runner.Allocated, err errors.Error) {

	rqst := runner.AllocRequest{
		Group: p.Group,
	}

	// Before continuing locate GPU resources for the task that has been received
	//
	var errGo error
	// The GPU values are optional and default to 0
	if 0 != len(p.Request.Experiment.Resource.GpuMem) {
		if rqst.MaxGPUMem, errGo = runner.ParseBytes(p.Request.Experiment.Resource.GpuMem); errGo != nil {
			msg := fmt.Sprintf("could not handle the gpuMem value %s", p.Request.Experiment.Resource.GpuMem)
			// TODO Add an output function here for Issues #4, https://github.com/SentientTechnologies/studio-go-runner/issues/4
			return nil, errors.Wrap(errGo, msg).With("stack", stack.Trace().TrimRuntime())
		}
	}

	rqst.MaxGPU = uint(p.Request.Experiment.Resource.Gpus)

	rqst.MaxCPU = uint(p.Request.Experiment.Resource.Cpus)
	if rqst.MaxMem, errGo = humanize.ParseBytes(p.Request.Experiment.Resource.Ram); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if rqst.MaxDisk, errGo = humanize.ParseBytes(p.Request.Experiment.Resource.Hdd); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if alloc, errGo = resources.AllocResources(rqst); errGo != nil {
		msg := fmt.Sprintf("alloc %s failed", Spew.Sdump(p.Request.Experiment.Resource))
		return nil, errors.Wrap(errGo, msg).With("stack", stack.Trace().TrimRuntime())
	}

	logger.Debug(fmt.Sprintf("alloc %s, gave %s", Spew.Sdump(rqst), Spew.Sdump(*alloc)))

	return alloc, nil
}

// deallocate first releases resources and then triggers a ready channel to notify any listener that the
func (p *processor) deallocate(alloc *runner.Allocated) {

	if errs := alloc.Release(); len(errs) != 0 {
		for _, err := range errs {
			logger.Warn(fmt.Sprintf("dealloc %s rejected due to %s", Spew.Sdump(*alloc), err.Error()))
		}
	} else {
		logger.Debug(fmt.Sprintf("released %s", Spew.Sdump(*alloc)))
	}

	// Only wait a second to alter others that the resources have been released
	//
	select {
	case <-time.After(time.Second):
	case p.ready <- true:
	}
}

// ProcessMsg is the main function where experiment processing occurs.
//
// This function blocks.
//
func (p *processor) Process(ctx context.Context) (wait time.Duration, ack bool, err errors.Error) {

	// Call the allocation function to get access to resources and get back
	// the allocation we received
	alloc, err := p.allocate()
	if err != nil {
		return errBackoff, false, errors.Wrap(err, "allocation fail backing off").With("stack", stack.Trace().TrimRuntime())
	}

	// Setup a function to release resources that have been allocated
	defer p.deallocate(alloc)

	// Use a panic handler to catch issues related to, or unrelated to the runner
	//
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic running studioml script %#v, %s", r, string(debug.Stack())))
		}
	}()

	// The allocation details are passed in to the runner to allow the
	// resource reservations to become known to the running applications.
	// This call will block until the task stops processing.
	if _, err = p.deployAndRun(ctx, alloc); err != nil {
		return time.Duration(0), true, err
	}

	return time.Duration(0), true, nil
}

// getHash produces a very simple and short hash for use in generating directory names from
// the experiment IDs assign by users to shorten the names and defang them
//
func getHash(text string) string {
	//	hasher := md5.New()
	//	hasher.Write([]byte(text))
	//	return hex.EncodeToString(hasher.Sum(nil))
	//
	// The stadtx hash could improve on this, see https://github.com/dgryski/go-stadtx.  However
	// it appears the impl was never set in stone and the author has disappeared from github
	//
	return fmt.Sprintf("%x", farm.Hash64([]byte(text)))
}

// mkUniqDir will create a working directory for an experiment
// using the file system calls appropriately so as to make sure
// no other instance of the same experiment is using it.  It is
// only being used by the caller and for which no race conditions
// during creation would have occurred.
//
// A new UUID could have been used to do this but that makes
// diagnosis of failed experiements very difficult so we keep a meaningful
// name for the new directory and use an index on the end of the experiment
// id so that during diagnosis we know exactly which attempts came first.
//
// There are lots of easier methods to create unique directories of course,
// but most involve using long unique identifies.
//
// This function will fill in the name being used into the structure being
// used for the method scope on success.
//
// The name of a created working directory will be returned that can be used
// for the dynamic portions of processing such as for creating
// the python virtual environment and also script files used by the runner.  This
// isolates experimenter supplied files from the runners working files and
// can be prevent uploading artifacts needlessly.
//
func (p *processor) mkUniqDir() (dir string, err errors.Error) {

	self, errGo := shortid.Generate()
	if errGo != nil {
		return dir, errors.Wrap(errGo, "generating a signature dir failed").With("stack", stack.Trace().TrimRuntime())
	}

	// Shorten any excessively massively long names supplied by users
	expDir := getHash(p.Request.Experiment.Key)

	inst := 0
	for {
		// Loop until we fail to find a directory with the prefix
		for {
			p.ExprDir = filepath.Join(p.RootDir, "experiments", expDir+"."+strconv.Itoa(inst))
			if _, errGo = os.Stat(p.ExprDir); errGo == nil {
				logger.Trace(fmt.Sprintf("found collision %s for %d", p.ExprDir, inst))
				inst++
				continue
			}
			break
		}

		// Create the next directory in sequence with another directory containing our signature
		if errGo = os.MkdirAll(filepath.Join(p.ExprDir, self), 0700); errGo != nil {
			p.ExprDir = ""
			return dir, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		logger.Trace(fmt.Sprintf("check for collision in %s", p.ExprDir))
		// After creation check to make sure our signature is the only file there, meaning no other entity
		// used the same experiment and instance
		files, errGo := ioutil.ReadDir(p.ExprDir)
		if errGo != nil {
			return dir, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		if len(files) != 1 {
			logger.Debug(fmt.Sprintf("looking in what should be a single file inside our experiment and find %s", Spew.Sdump(files)))
			// Increment the instance for the next pass
			inst++

			// Backoff for a small amount of time, less than a second then attempt again
			<-time.After(time.Duration(rand.Intn(1000)) * time.Millisecond)
			logger.Debug(fmt.Sprintf("collision during creation of %s with %d files", p.ExprDir, len(files)))
			continue
		}
		p.ExprSubDir = expDir + "." + strconv.Itoa(inst)

		os.Remove(filepath.Join(p.ExprDir, self))
		return "", nil
	}
}

// extractValidEnv is used to convert the environment variables of the current process
// into a map removing any names that dont translate to valid user environment variables,
// such as names that start with underscores etc
//
func extractValidEnv() (envs map[string]string) {

	envs = map[string]string{}
	for _, v := range os.Environ() {
		// After the first equal keep everything else together
		kv := strings.SplitN(v, "=", 2)
		// Extract the first unicode rune and test that it is a valid character for an env name
		envName := []rune(kv[0])
		if len(kv) == 2 && (unicode.IsLetter(envName[0]) || unicode.IsDigit(envName[0])) {
			kv[1] = strings.Replace(kv[1], "\"", "\\\"", -1)
			envs[kv[0]] = kv[1]
		} else {
			// The underscore is always present and represents the CWD so dont print messages about it
			if envName[0] != '_' {
				logger.Debug(fmt.Sprintf("env var %s (%c) (%d) dropped due to conformance", kv[0], envName[0], len(kv)))
			}
		}
	}
	return envs
}

// applyEnv is used to apply the contents of the env block specified by the studioml client into the
// runners environment table.
//
// this function is also used to examine the contents of the processor request environment variables and
// to resolve locally any environment variables that are present indicated by the %...% pairs.
// If the enclosed value is not an environment variable within the context of the runner then the
// text will be left untouched.
//
// This behavior is specific to the go runner at this time.
//
func (p *processor) applyEnv(alloc *runner.Allocated) {

	p.ExprEnvs = extractValidEnv()

	// Expand %...% pairs by iterating the env table for the process and explicitly replacing on each line
	re := regexp.MustCompile(`(?U)(?:\%(.*)*\%)+`)

	// Environment variables need to be applied here to assist in unpacking S3 files etc
	for k, v := range p.Request.Config.Env {

		for _, match := range re.FindAllString(v, -1) {
			if envV := os.Getenv(match[1 : len(match)-1]); len(envV) != 0 {
				v = strings.Replace(envV, match, envV, -1)
			}
		}
		// Update the processor env table with the resolved value
		p.Request.Config.Env[k] = v

		p.ExprEnvs[k] = v
	}
	// create the map into which customer environment variables will be added to
	// the experiment script
	//
	p.ExprEnvs["AWS_SDK_LOAD_CONFIG"] = "1"

	// Although we copy the env values to the runners env table through they done get
	// automatically included into the script this is done via the Make being given
	// a set of env variables as an array that will be written into the script using the receiever
	// contents.
	//
	if alloc.GPU != nil && len(alloc.GPU.Env) != 0 {
		for k, v := range alloc.GPU.Env {
			p.ExprEnvs[k] = v
		}
	}
}

func (p *processor) calcTimeLimit() (maxDuration time.Duration) {
	// Determine when the life time of the experiment is over and then check it before starting
	// the experiment.  when running this function also checks to ensure the lifetime has not expired
	//
	maxDuration = time.Duration(96 * time.Hour)
	if len(p.Request.Config.Lifetime) != 0 {
		limit, errGo := time.ParseDuration(p.Request.Config.Lifetime)
		if errGo != nil {
			msg := fmt.Sprintf("%s %s maximum life time ignored due to %v", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, errGo)
			runner.WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
		} else {
			if p.Request.Experiment.TimeAdded > 10.0 {
				limit = time.Until(time.Unix(int64(p.Request.Experiment.TimeAdded), 0).Add(limit))
				if limit <= 0 {
					msg := fmt.Sprintf("%s %s maximum life time reached", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key)
					runner.WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
					return 0
				}
				if limit < maxDuration {
					maxDuration = limit
				}
			}
		}
	}

	// Determine the maximum run duration for any single attempt to run the experiment
	if len(p.Request.Experiment.MaxDuration) != 0 {
		limit, errGo := time.ParseDuration(p.Request.Experiment.MaxDuration)
		if errGo != nil {
			msg := fmt.Sprintf("%s %s maximum duration ignored due to %v", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, errGo)
			runner.WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
		}
		if limit < maxDuration {
			maxDuration = limit
		}
	}
	return maxDuration
}

func (p *processor) doOutput(refresh map[string]runner.Artifact) {
	for group, artifact := range refresh {
		p.returnOne(group, artifact)
	}
}

func (p *processor) checkpointOutput(refresh map[string]runner.Artifact, quitC chan bool) (doneC chan bool) {
	doneC = make(chan bool, 1)

	disableCP := true
	// On a regular basis we will flush the log and compress it for uploading to
	// AWS or Google Cloud Storage etc, use the interval specified in the meta data for the job
	//
	saveDuration := time.Duration(600 * time.Minute)
	if len(p.Request.Config.SaveWorkspaceFrequency) > 0 {
		duration, errGo := time.ParseDuration(p.Request.Config.SaveWorkspaceFrequency)
		if errGo == nil {
			if duration > time.Duration(time.Second) && duration < time.Duration(12*time.Hour) {
				saveDuration = duration
				disableCP = false
			}
		} else {
			msg := fmt.Sprintf("%s %s save workspace frequency ignored due to %v", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, errGo)
			runner.WarningSlack(p.Request.Config.Runner.SlackDest, msg, []string{})
		}
	}

	go func() {

		defer close(doneC)

		checkpoint := time.NewTicker(saveDuration)
		defer checkpoint.Stop()
		for {
			select {
			case <-checkpoint.C:

				if disableCP {
					continue
				}

				p.doOutput(refresh)
			case <-quitC:
				return
			}
		}
	}()
	return doneC
}

func (p *processor) runScript(ctx context.Context, refresh map[string]runner.Artifact) (err errors.Error) {

	quitC := make(chan bool)

	// Start a checkpointer for our output files and pass it the channel used
	// to notify when it is to stop.  Save a reference to the channel used to
	// indicate when the checkpointer has flushed files etc.
	//
	// This function also ensures that the queue related to the work being
	// processed is still present, if not the task should be terminated.
	//
	doneC := p.checkpointOutput(refresh, quitC)

	// Blocking call to run the process that uses the ctx for timeouts etc
	err = p.Executor.Run(ctx, refresh)

	// Notify the checkpointer that things are done with
	close(quitC)

	// Make sure any checkpointing is done before continuing to handle results
	// and artifact uploads
	<-doneC

	return err
}

func (p *processor) run(ctx context.Context, alloc *runner.Allocated) (err errors.Error) {

	logger.Debug("starting run")
	defer logger.Debug("stopping run")

	// Now figure out the absolute time that the experiment is limited to
	maxDuration := p.calcTimeLimit()
	terminateAt := time.Now().Add(maxDuration)

	if terminateAt.Before(time.Now()) {
		msg := fmt.Sprintf("%s %s elapsed limit %s has already expired at %s", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, maxDuration.String(), terminateAt.Local().String())
		return errors.New(msg).With("stack", stack.Trace().TrimRuntime()).With("request", *p.Request)
	}

	if logger.IsTrace() {

		files := []string{}
		searchDir := path.Dir(p.ExprDir)
		filepath.Walk(searchDir, func(path string, f os.FileInfo, err error) error {
			files = append(files, path)
			return nil
		})
		logger.Trace("on disk manifest", "dir", searchDir, "files", strings.Join(files, ", "))
	}

	fmt.Printf("alloc sent to Make is %+v\n", alloc.GPU)
	// Now we have the files locally stored we can begin the work
	if err = p.Executor.Make(alloc, p); err != nil {
		return err
	}

	refresh := make(map[string]runner.Artifact, len(p.Request.Experiment.Artifacts))
	for k, v := range p.Request.Experiment.Artifacts {
		if v.Mutable {
			refresh[k] = v
		}
	}

	// Recheck the expiry time as the make step can be time consuming
	if terminateAt.Before(time.Now()) {
		msg := fmt.Sprintf("%s %s has already expired at %s", p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, terminateAt.Local().String())
		logger.Info(msg)
		return errors.New(msg).With("stack", stack.Trace().TrimRuntime())
	}

	logger.Debug(fmt.Sprintf("%s %s lifetime set to %s (%s) (%s)", p.Request.Config.Database.ProjectId,
		p.Request.Experiment.Key, terminateAt.Local().String(), p.Request.Config.Lifetime, p.Request.Experiment.MaxDuration))

	// Setup a timelimit for the work we are doing
	startTime := time.Now()
	runCtx, runCancel := context.WithTimeout(context.Background(), maxDuration)
	// Always cancel the operation, however we should ignore errors as these could
	// be already cancelled so we need to ignore errors at this point
	defer func() {
		defer func() {
			recover()
		}()
		runCancel()
	}()

	// If the outer context gets cancelled cancel our inner context
	go func() {
		select {
		case <-ctx.Done():
			logger.Debug(fmt.Sprintf("%s %s stopped by processor client after %s",
				p.Request.Config.Database.ProjectId, p.Request.Experiment.Key, time.Since(startTime)))
			runCancel()
		}
	}()

	// Blocking call to run the script and only return when done.  Cancellation is done
	// if needed using the cancel function created by the context
	err = p.runScript(runCtx, refresh)

	// Send any output to the slack reporter
	p.slackOutput()

	return err
}

func outputErr(fn string, inErr errors.Error) (err errors.Error) {
	if inErr == nil {
		return nil
	}
	f, errGo := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()
	f.WriteString("failed when downloading user data\n")
	f.WriteString(fmt.Sprintf("%+v\n", errors.Wrap(inErr).With("stack", stack.Trace().TrimRuntime())))
	return nil
}

// deployAndRun is called to execute the work unit
//
func (p *processor) deployAndRun(ctx context.Context, alloc *runner.Allocated) (warns []errors.Error, err errors.Error) {

	uploaded := false

	defer func() {
		if !uploaded {
			//We should always upload results even in the event of an error to
			// help give the experimenter some clues as to what might have
			// failed if there is a problem
			p.returnAll()
		}

		if !*debugOpt {
			defer os.RemoveAll(p.ExprDir)
		}
	}()

	// Update and apply environment variables for the experiment
	p.applyEnv(alloc)

	if *debugOpt {
		// The following log can expose passwords etc.  As a result we do not allow it unless the debug
		// non production flag is explicitly set
		logger.Trace(fmt.Sprintf("experiment → %v → %s → %#v", p.Request.Experiment, p.ExprDir, *p.Request))
	}

	// The standard output file for studio jobs, is used here in the event that a catastropic error
	// occurs before the job starts
	//
	outputFN := filepath.Join(p.ExprDir, "output", "output")

	// fetchAll when called will have access to the environment variables used by the experiment in order that
	// credentials can be used
	if err = p.fetchAll(); err != nil {
		// A failure here should result in a warning being written to the processor
		// output file in the hope that it will be returned.  Likewise further on down in
		// this function
		//
		if errO := outputErr(outputFN, err); errO != nil {
			warns = append(warns, errO)
		}
		return warns, err
	}

	// Blocking call to run the task
	if err = p.run(ctx, alloc); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		// TODO: If the failure was related to the healthcheck then requeue and backoff the queue
		if errO := outputErr(outputFN, err); errO != nil {
			warns = append(warns, errO)
		}
		return warns, err
	}

	uploaded = true

	return p.returnAll()
}
