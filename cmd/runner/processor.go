package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/valyala/fastjson"

	"github.com/dgryski/go-farm"

	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/dustin/go-humanize"
	"github.com/karlmutch/base62"
	"github.com/karlmutch/go-shortid"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
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
	// caching kv.that might be short lived
	cacheReport sync.Once
)

func init() {
	res, err := runner.NewResources(*tempOpt)
	if err != nil {
		logger.Fatal("could not initialize disk space tracking", "err", err.Error())
	}
	resources = res

	// A cache exists on linux for cuda lets remove it as it
	// can cause issues
	errGo := os.RemoveAll("$HOME/.nv")
	if errGo != nil {
		logger.Fatal("could not clear the $HOME/.nv cache", "err", err.Error())
	}
}

func cacheReporter(ctx context.Context) {
	for {
		select {
		case err := <-artifactCache.ErrorC:
			logger.Info("artifact cache error", "error", err, "stack", stack.Trace().TrimRuntime())
		case <-ctx.Done():
			return
		}
	}
}

// Executor is an interface that defines a job handling worker implementation.  Each variant of a worker
// conforms to a standard processor interface
//
type Executor interface {

	// Make is used to allow a script to be generated for the specific run strategy being used
	Make(alloc *runner.Allocated, e interface{}) (err kv.Error)

	// Run will execute the worker task used by the experiment
	Run(ctx context.Context, refresh map[string]runner.Artifact) (err kv.Error)

	// Close can be used to tidy up after an experiment has completed
	Close() (err kv.Error)
}

// newProcessor will create a new working directory
//
func newProcessor(ctx context.Context, group string, msg []byte, creds string) (proc *processor, err kv.Error) {

	// When a processor is initialized make sure that the logger is enabled first time through
	//
	cacheReport.Do(func() {
		go cacheReporter(ctx)
	})

	temp, err := func() (temp string, err kv.Error) {
		// Singleton style initialization to instantiate and overridding directory
		// for the entire server working area
		//
		tempRoot.Lock()
		defer tempRoot.Unlock()

		if tempRoot.dir == "" {
			id, errGo := shortid.Generate()
			if errGo != nil {
				return "", kv.Wrap(errGo, "temp file id generation failed").With("stack", stack.Trace().TrimRuntime())
			}
			if tempRoot.dir, errGo = ioutil.TempDir(*tempOpt, "gorun_"+id); errGo != nil {
				return "", kv.Wrap(errGo, "temp file create failed").With("stack", stack.Trace().TrimRuntime())
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
		return nil, kv.NewError("unable to determine execution class from artifacts").With("stack", stack.Trace().TrimRuntime()).
			With("project", p.Request.Config.Database.ProjectId).With("experiment", p.Request.Experiment.Key)
	}

	logger.Info("experiment initialized", "dir", p.ExprDir, "stack", stack.Trace().TrimRuntime())

	return p, nil
}

const (
	// ExecUnknown is an unused guard value
	ExecUnknown = iota
	// ExecPythonVEnv indicates we are using the python virtualenv packaging
	ExecPythonVEnv
	// ExecSingularity inidcates we are using the Singularity container packaging and runtime
	ExecSingularity
)

// Close will release all resources and clean up the work directory that
// was used by the studioml work
//
func (p *processor) Close() (err error) {
	if *debugOpt || 0 == len(p.ExprDir) {
		logger.Info("experiment kept", "dir", p.ExprDir, "stack", stack.Trace().TrimRuntime())
		return nil
	}

	logger.Info("experiment removed", "dir", p.ExprDir, "stack", stack.Trace().TrimRuntime())
	return os.RemoveAll(p.ExprDir)
}

// fetchAll is used to retrieve from the storage system employed by studioml any and all available
// artifacts and to unpack them into the experiment directory
//
func (p *processor) fetchAll(ctx context.Context) (err kv.Error) {

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
		warns, err := artifactCache.Fetch(ctx, &artifact, p.Request.Config.Database.ProjectId, group, p.Creds, p.ExprEnvs, p.ExprDir)

		if err != nil {
			msg := "artifact fetch failed"
			msgDetail := []interface{}{
				"group", group,
				"project", p.Request.Config.Database.ProjectId,
				"Experiment", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime(),
				"err", err,
			}
			if artifact.Mutable {
				logger.Debug(msg, msgDetail)
			} else {
				logger.Warn(msg, msgDetail)
			}
			msgDetail[len(msgDetail)-2] = "warning"
			for _, warn := range warns {
				msgDetail[len(msgDetail)-1] = warn
				if artifact.Mutable {
					logger.Debug(msg, msgDetail)
				} else {
					logger.Warn(msg, msgDetail)
				}
			}

			// Mutable artifacts can be create only items that dont yet exist on the storage platform
			if !artifact.Mutable {
				return err.With(msgDetail...)
			}
		}
	}
	return nil
}

// copyToMetaData is used to copy a file to the meta data area using the file naming semantics
// of the metadata layout
func (p *processor) copyToMetaData(src string, dest string, jsonDest string) (err kv.Error) {

	logger.Info("copying", "source", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())

	fStat, errGo := os.Stat(src)
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}

	if !fStat.Mode().IsRegular() {
		return kv.NewError("not a regular file").With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}

	source, errGo := os.Open(filepath.Clean(src))
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	defer source.Close()

	destination, errGo := os.OpenFile(dest, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		destination.Close()

		// Uploading a zero length file is pointless
		if fileInfo, errGo := os.Stat(dest); errGo == nil {
			if fileInfo.Size() == 0 {
				_ = os.Remove(dest)
			}
		}
	}()

	// If there is no need to scan the file look for json data to scrape from it
	// simply copy the file and return
	if len(jsonDest) == 0 {
		if _, errGo = io.Copy(destination, source); errGo != nil {
			return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
		}
		return nil
	}

	// If we need to scrape the file then we should scan it line by line
	jsonDestination, errGo := os.OpenFile(jsonDest, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		jsonDestination.Close()

		// Uploading a zero length file is pointless
		if fileInfo, errGo := os.Stat(jsonDest); errGo == nil {
			if fileInfo.Size() == 0 {
				_ = os.Remove(jsonDest)
			}
		}
	}()

	// Store any discovered json fragments for generating experiment documents as a single collection
	jsonDirectives := []string{}

	// Checkmarx code checking note. Checkmarx is for Web applications and is not a good fit general purpose server code.
	// It is also worth mentioning that if you are reading this message that Checkmarx does not understand Go package structure
	// and does not appear to use the Go AST  to validate code so is not able to perform path and escape analysis which
	// means that more than 95% of warning are for unvisited code.
	//
	// The following will raise a 'Denial Of Service Resource Exhaustion' message but this is bogus.
	// The scanner in go is space limited intentially to prevent resource exhaustion.
	s := bufio.NewScanner(source)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		if _, errGo = fmt.Fprintln(destination, s.Text()); errGo != nil {
			return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
		}
		line := strings.TrimSpace(s.Text())
		if len(line) <= 2 {
			continue
		}
		if (line[0] != '{' || line[len(line)-1] != '}') &&
			(line[0] != '[' || line[len(line)-1] != ']') {
			continue
		}
		// After each line is scanned the json fragment is merged into a collection of all detected patches and merges that
		// have been output by the experiment
		if errGo = fastjson.Validate(line); errGo != nil {
			if logger.IsTrace() {
				logger.Trace("output json filter failed", "error", errGo, "line", line, "stack", stack.Trace().TrimRuntime())
			}
			continue
		}
		jsonDirectives = append(jsonDirectives, line)
		if logger.IsTrace() {
			logger.Debug("json filter added", "line", line, "stack", stack.Trace().TrimRuntime())
		}
	}
	if len(jsonDirectives) == 0 {
		return nil
	}
	result, err := runner.JSONEditor("", jsonDirectives)
	if err != nil {
		return err
	}

	if _, errGo = fmt.Fprintln(jsonDestination, result); errGo != nil {
		return kv.Wrap(errGo).With("src", src, "dest", dest, "jsonDest", jsonDest, "stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// updateMetaData is used to update files and artifacts related to the experiment
// that reside in the meta data area
//
func (p *processor) updateMetaData(group string, artifact runner.Artifact, accessionID string) (err kv.Error) {

	metaDir := filepath.Join(p.ExprDir, "_metadata")
	if _, errGo := os.Stat(metaDir); os.IsNotExist(errGo) {
		os.MkdirAll(metaDir, 0700)
	}

	switch group {
	case "output":
		src := filepath.Join(p.ExprDir, "output", "output")
		dest := filepath.Join(metaDir, "output-host-"+accessionID+".log")
		jsonDest := filepath.Join(metaDir, "scrape-host-"+accessionID+".json")
		return p.copyToMetaData(src, dest, jsonDest)
	default:
		return kv.NewError("group unrecognized").With("group", group, "stack", stack.Trace().TrimRuntime())
	}
}

// returnOne is used to upload a single artifact to the data store specified by the experimenter
//
func (p *processor) returnOne(ctx context.Context, group string, artifact runner.Artifact, accessionID string) (uploaded bool, warns []kv.Error, err kv.Error) {

	// Meta data is specialized
	if len(accessionID) != 0 {
		switch group {
		case "output":
			if err = p.updateMetaData(group, artifact, accessionID); err != nil {
				logger.Warn("output artifact could not be used for metadata", "project_id", p.Request.Config.Database.ProjectId,
					"experiment_id", p.Request.Experiment.Key, "error", err.Error())
			}
		}
	}

	uploaded, warns, err = artifactCache.Restore(ctx, &artifact, p.Request.Config.Database.ProjectId, group, p.Creds, p.ExprEnvs, p.ExprDir)
	if err != nil {
		logger.Warn("artifact could not be returned", "project_id", p.Request.Config.Database.ProjectId,
			"experiment_id", p.Request.Experiment.Key, "artifact", artifact, "error", err.Error())
	}
	return uploaded, warns, err
}

// returnAll creates tar archives of the experiments artifacts and then puts them
// back to the studioml shared storage
//
func (p *processor) returnAll(ctx context.Context, accessionID string) (warns []kv.Error, err kv.Error) {

	returned := make([]string, 0, len(p.Request.Experiment.Artifacts))

	// Accessioning can modify the system artifacts and so the order we traverse
	// is important, we want the _metadata artifact after the _output
	// artifact which can be done using a descending sort which places underscores
	// before lowercase letters
	//
	keys := make([]string, 0, len(p.Request.Experiment.Artifacts))
	for group := range p.Request.Experiment.Artifacts {
		keys = append(keys, group)
	}
	sort.Sort(sort.StringSlice(keys))

	for _, group := range keys {
		if artifact, isPresent := p.Request.Experiment.Artifacts[group]; isPresent {
			if artifact.Mutable {
				if _, warns, err = p.returnOne(ctx, group, artifact, accessionID); err != nil {
					return warns, err
				}
			}
		}
	}

	if len(returned) != 0 {
		logger.Info("project returning", "project_id", p.Request.Config.Database.ProjectId, "result", strings.Join(returned, ", "))
	}

	return warns, nil
}

// allocate is used to reserve the resources on the local host needed to handle the entire job as
// a highwater mark.
//
// The returned alloc structure should be used with the deallocate function otherwise resource
// leaks will occur.
//
func (p *processor) allocate() (alloc *runner.Allocated, err kv.Error) {

	rqst := runner.AllocRequest{}

	// Before continuing locate GPU resources for the task that has been received
	//
	var errGo error
	// The GPU values are optional and default to 0
	if 0 != len(p.Request.Experiment.Resource.GpuMem) {
		if rqst.MaxGPUMem, errGo = runner.ParseBytes(p.Request.Experiment.Resource.GpuMem); errGo != nil {
			// TODO Add an output function here for Issues #4, https://github.com/leaf-ai/studio-go-runner/issues/4
			return nil, kv.Wrap(errGo, "gpuMem value is invalid").With("gpuMem", p.Request.Experiment.Resource.GpuMem).With("stack", stack.Trace().TrimRuntime())
		}
	}

	rqst.MaxGPU = uint(p.Request.Experiment.Resource.Gpus)

	rqst.MaxCPU = uint(p.Request.Experiment.Resource.Cpus)
	if rqst.MaxMem, errGo = humanize.ParseBytes(p.Request.Experiment.Resource.Ram); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if rqst.MaxDisk, errGo = humanize.ParseBytes(p.Request.Experiment.Resource.Hdd); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if alloc, err = resources.AllocResources(rqst); err != nil {
		return nil, err
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
func (p *processor) Process(ctx context.Context) (wait time.Duration, ack bool, err kv.Error) {

	host, _ := os.Hostname()
	accessionID := host + "-" + base62.EncodeInt64(time.Now().Unix())

	// Call the allocation function to get access to resources and get back
	// the allocation we received
	alloc, err := p.allocate()
	if err != nil {
		return errBackoff, false, kv.Wrap(err, "allocation fail backing off").With("stack", stack.Trace().TrimRuntime())
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
	if _, err = p.deployAndRun(ctx, alloc, accessionID); err != nil {
		return time.Duration(10 * time.Second), false, err
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
func (p *processor) mkUniqDir() (dir string, err kv.Error) {

	self, errGo := shortid.Generate()
	if errGo != nil {
		return dir, kv.Wrap(errGo, "generating a signature dir failed").With("stack", stack.Trace().TrimRuntime())
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
			return dir, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		logger.Trace(fmt.Sprintf("check for collision in %s", p.ExprDir))
		// After creation check to make sure our signature is the only file there, meaning no other entity
		// used the same experiment and instance
		files, errGo := ioutil.ReadDir(p.ExprDir)
		if errGo != nil {
			return dir, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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

	// Checkmarx code checking note. Checkmarx is for Web applications and is not a good fit general purpose server code.
	// It is also worth mentioning that if you are reading this message that Checkmarx does not understand Go package structure
	// and does not appear to use the Go AST  to validate code so is not able to perform path and escape analysis which
	// means that more than 95% of warning are for unvisited code.
	//
	// The following will raise a 'Denial Of Service Resource Exhaustion' message but this is bogus.
	// The data used to generate the p.Request values comes from a managed json message that is validated.

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
	for _, gpu := range alloc.GPU {
		for env, gpuVar := range gpu.Env {
			if len(gpuVar) != 0 {
				if expVar, isPresent := p.ExprEnvs[env]; isPresent {
					p.ExprEnvs[env] = expVar + "," + gpuVar
				} else {
					p.ExprEnvs[env] = gpuVar
				}
			}
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
			logger.Warn("maximum life time ignored", "error", errGo,
				"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime())
		} else {
			if p.Request.Experiment.TimeAdded > 10.0 {
				limit = time.Until(time.Unix(int64(p.Request.Experiment.TimeAdded), 0).Add(limit))
				if limit <= 0 {
					logger.Warn("maximum life time reached",
						"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
						"stack", stack.Trace().TrimRuntime())
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
			logger.Warn("maximum duration ignored", "error", errGo,
				"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime())
		}
		if limit < maxDuration {
			maxDuration = limit
		}
	}
	return maxDuration
}

func (p *processor) checkpointStart(ctx context.Context, accessionID string, refresh map[string]runner.Artifact, saveTimeout time.Duration) (doneC chan struct{}) {
	doneC = make(chan struct{}, 1)

	// On a regular basis we will flush the log and compress it for uploading to
	// AWS or Google Cloud Storage etc, use the interval specified in the meta data for the job
	//
	saveDuration := time.Duration(600 * time.Minute)
	if len(p.Request.Config.SaveWorkspaceFrequency) > 0 {
		duration, errGo := time.ParseDuration(p.Request.Config.SaveWorkspaceFrequency)
		if errGo == nil {
			if duration > time.Duration(2*time.Minute) && duration < time.Duration(12*time.Hour) {
				saveDuration = duration
			}
		} else {
			logger.Warn("save workspace frequency ignored", "error", errGo,
				"project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"stack", stack.Trace().TrimRuntime())
		}
	}

	go p.checkpointer(ctx, saveDuration, saveTimeout, accessionID, refresh, doneC)

	return doneC
}

// checkpointArtifacts will run through the artifacts within a refresh list
// and make sure they are all commited to the data store used by the
// experiment
func (p *processor) checkpointArtifacts(ctx context.Context, accessionID string, refresh map[string]runner.Artifact) {
	for group, artifact := range refresh {
		p.returnOne(ctx, group, artifact, accessionID)
	}
}

// checkpointer is designed to take items such as progress tracking artifacts and on a regular basis
// save these to the artifact store while the experiment is running.  The refresh collection contains
// a list of the artifacts that need to be checkpointed
//
func (p *processor) checkpointer(ctx context.Context, saveInterval time.Duration, saveTimeout time.Duration, accessionID string, refresh map[string]runner.Artifact, doneC chan struct{}) {

	defer close(doneC)

	checkpoint := time.NewTicker(saveInterval)
	defer checkpoint.Stop()

	for {
		select {
		case <-checkpoint.C:
			// The context that is supplied by the caller relates to the experiment itself, however what we dont want
			// to happen is for the uploading of artifacts to be terminated until they complete so we build a new context
			// for the uploads and use the ctx supplied as a lifecycle indicator
			uploadCtx, uploadCancel := context.WithTimeout(context.Background(), saveTimeout)

			// Here a regular checkpoint of the artifacts is being done.  Before doing this
			// we should copy meta data related files from the output directory and other
			// locations into the _metadata artifact area
			p.checkpointArtifacts(uploadCtx, accessionID, refresh)
			uploadCancel()

		case <-ctx.Done():
			// The context that is supplied by the caller relates to the experiment itself, however what we dont want
			// to happen is for the uploading of artifacts to be terminated until they complete so we build a new context
			// for the uploads and use the ctx supplied as a lifecycle indicator
			uploadCtx, uploadCancel := context.WithTimeout(context.Background(), saveTimeout)
			defer uploadCancel()

			// The context can be canncelled externally in which case
			// we should still push any changes that occured since the last
			// checkpoint
			p.checkpointArtifacts(uploadCtx, accessionID, refresh)
			return
		}
	}
}

// runScript is used to start a script execution along with an artifact checkpointer that both remain running until the
// experiment is done.  refresh contains a list of the artifacts that require checkpointing
//
func (p *processor) runScript(ctx context.Context, accessionID string, refresh map[string]runner.Artifact, refreshTimeout time.Duration) (err kv.Error) {

	// Create a context that can be cancelled within the runScript so that the checkpointer
	// and the executor are aligned on the termination of a job either from the base
	// context that would normally be a timeout or explicit cancellation, or the task
	// completes normally and terminates by returning
	runCtx, runCancel := context.WithCancel(ctx)

	// Start a checkpointer for our output files and pass it the channel used
	// to notify when it is to stop.  Save a reference to the channel used to
	// indicate when the checkpointer has flushed files etc.
	//
	// This function also ensures that the queue related to the work being
	// processed is still present, if not the task should be terminated.
	//
	doneC := p.checkpointStart(runCtx, accessionID, refresh, refreshTimeout)

	// Blocking call to run the process that uses the ctx for timeouts etc
	err = p.Executor.Run(runCtx, refresh)

	// When the runner itself stops then we can cancel the context which will signal the checkpointer
	// to do one final save of the experiment data and return after closing its own doneC channel
	runCancel()

	// Make sure any checkpointing is done before continuing to handle results
	// and artifact uploads
	<-doneC

	return err
}

func (p *processor) run(ctx context.Context, alloc *runner.Allocated, accessionID string) (err kv.Error) {

	// Now figure out the absolute time that the experiment is limited to
	maxDuration := p.calcTimeLimit()
	startedAt := time.Now()
	terminateAt := time.Now().Add(maxDuration)

	if terminateAt.Before(time.Now()) {
		return kv.NewError("elapsed limit has expired").
			With("project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"started_at", startedAt, "max_duration", maxDuration.String(),
				"request", *p.Request).
			With("stack", stack.Trace().TrimRuntime())
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
	// This value is not yet parameterized but should eventually be
	//
	// Each checkpoint or artifact upload back to the s3 servers etc needs a timeout that is
	// seperate from the go context for the experiment in order that when timeouts occur on
	// the experiment they dont trash artifact uploads which are permitted to run after the
	// experiment has terminated/stopped/killed etc
	//
	refreshTimeout := 5 * time.Minute

	// Recheck the expiry time as the make step can be time consuming
	if terminateAt.Before(time.Now()) {
		return kv.NewError("already expired").
			With("project_id", p.Request.Config.Database.ProjectId, "experiment_id", p.Request.Experiment.Key,
				"started_at", startedAt, "max_duration", maxDuration.String(),
				"stack", stack.Trace().TrimRuntime())
	}

	// Setup a timelimit for the work we are doing
	runCtx, runCancel := context.WithTimeout(ctx, maxDuration)
	defer runCancel()

	if logger.IsInfo() {

		deadline, _ := runCtx.Deadline()

		logger.Info("starting run",
			"project_id", p.Request.Config.Database.ProjectId,
			"experiment_id", p.Request.Experiment.Key,
			"lifetime_duration", p.Request.Config.Lifetime,
			"started_at", startedAt,
			"max_duration", p.Request.Experiment.MaxDuration,
			"deadline", deadline,
			"stack", stack.Trace().TrimRuntime())
		defer logger.Debug("stopped run",
			"started_at", startedAt,
			"stack", stack.Trace().TrimRuntime())
	}

	// Blocking call to run the script and only return when done.  Cancellation is done
	// if needed using the cancel function created by the context, runCtx
	//
	return p.runScript(runCtx, accessionID, refresh, refreshTimeout)
}

func outputErr(fn string, inErr kv.Error) (err kv.Error) {
	if inErr == nil {
		return nil
	}
	f, errGo := os.OpenFile(fn, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()
	f.WriteString("failed when downloading user data\n")
	f.WriteString(inErr.Error())
	return nil
}

// deployAndRun is called to execute the work unit
//
func (p *processor) deployAndRun(ctx context.Context, alloc *runner.Allocated, accessionID string) (warns []kv.Error, err kv.Error) {

	defer func() {
		// We should always upload results even in the event of an error to
		// help give the experimenter some clues as to what might have
		// failed if there is a problem
		p.returnAll(ctx, accessionID)

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

	// The standard output file for studio jobs, is used here in the event that a catastrophic error
	// occurs before the job starts
	//
	outputFN := filepath.Join(p.ExprDir, "output", "output")

	// fetchAll when called will have access to the environment variables used by the experiment in order that
	// credentials can be used
	if err = p.fetchAll(ctx); err != nil {
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
	if err = p.run(ctx, alloc, accessionID); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		// TODO: If the failure was related to the healthcheck then requeue and backoff the queue
		if errO := outputErr(outputFN, err); errO != nil {
			warns = append(warns, errO)
		}
		return warns, err
	}
	return warns, err
}
