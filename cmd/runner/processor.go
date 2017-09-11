package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/pubsub"

	"github.com/ryankurte/go-async-cmd"
	"github.com/satori/go.uuid"

	"github.com/SentientTechnologies/studio-go-runner"
	"github.com/davecgh/go-spew/spew"

	"github.com/dustin/go-humanize"
)

type processor struct {
	Group      string            `json:"group"` // A caller specific grouping for work that can share sensitive resources
	RootDir    string            `json:"root_dir"`
	ExprDir    string            `json:"expr_dir"`
	ExprSubDir string            `json:"expr_sub_dir"`
	ExprEnvs   map[string]string `json:"expr_envs"`
	Request    *runner.Request   `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
	ready      chan bool         // Used by the processor to indicate it has released resources or state has changed
}

var (
	// This is a safety valve for when work should not be scheduled due to allocation
	// failing to get resources.  In these case we wait for another job to complete however
	// this might no occur for sometime and we might want to come back around and see
	// if a smaller job is available.  But we only do this after a backoff period to not
	// hammer queues relentlessly
	//
	errBackoff = time.Duration(5 * time.Minute)

	resources = &runner.Resources{}
)

func init() {
	res, err := runner.NewResources(*tempOpt)
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not initialize disk space tracking due to %s", err.Error()))
	}
	resources = res
}

// newProcessor will create a new working directory
//
func newProcessor(group string, msg *pubsub.Message) (p *processor, err error) {

	p = &processor{
		Group: group,
		ready: make(chan bool),
	}

	// Get a location for running the test
	//
	p.RootDir, err = ioutil.TempDir(*tempOpt, uuid.NewV4().String())
	if err != nil {
		return nil, err
	}

	p.Request, err = runner.UnmarshalRequest(msg.Data)
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Close will release all resources and clean up the work directory that
// was used by the studioml work
//
func (p *processor) Close() (err error) {
	if *debugOpt {
		return nil
	}

	return os.RemoveAll(p.RootDir)
}

// makeScript is used to write a script file that is generated for the specific TF tasks studioml has sent
// to retrieve any python packages etc then to run the task
//
func (p *processor) makeScript(fn string) (err error) {

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, err := template.New("pythonRunner").Parse(
		`#!/bin/bash
{{range $key, $value := .Request.Config.Env}}
export {{$key}}="{{$value}}"
{{end}}
{{range $key, $value := .ExprEnvs}}
export {{$key}}="{{$value}}"
{{end}}
export LD_LIBRARY_PATH=$LD_LIBRARY_PATH:/usr/local/cuda/lib64/
mkdir {{.RootDir}}/blob-cache
mkdir {{.RootDir}}/queue
mkdir {{.RootDir}}/artifact-mappings
mkdir {{.RootDir}}/artifact-mappings/{{.Request.Experiment.Key}}
cd {{.ExprDir}}/workspace
virtualenv --system-site-packages .
source bin/activate
{{range .Request.Config.Pip}}
pip install {{.}}
{{end}}
{{range .Request.Experiment.Pythonenv}}
{{if ne . "studioml=="}}pip install {{.}}{{end}}
{{end}}
if [ "` + "`" + `echo dist/studioml-*.tar.gz` + "`" + `" != "dist/studioml-*.tar.gz" ]; then
    pip install dist/studioml-*.tar.gz
else
    pip install studioml
fi
export STUDIOML_EXPERIMENT={{.ExprSubDir}}
export STUDIOML_HOME={{.RootDir}}
python {{.Request.Experiment.Filename}} {{range .Request.Experiment.Args}}{{.}} {{end}}
deactivate
`)

	if err != nil {
		return err
	}

	content := new(bytes.Buffer)
	err = tmpl.Execute(content, p)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fn, content.Bytes(), 0744)
}

// runScript will use a generated script file and will run it to completion while marshalling
// results and files from the computation
//
func (p *processor) runScript(ctx context.Context, fn string, refresh map[string]runner.Modeldir) (err error) {

	cmd := gocmd.Command("/bin/bash", "-c", fn)

	cmd.OutputChan = make(chan string, 1024)

	f, err := os.Create(filepath.Join(p.ExprDir, "output", "output"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Start the command handling everything else on channels
	cmd.Start()

	// On a regular basis we will flush the log and compress it for uploading to
	// firebase, use the interval specified in the meta data for the job
	//
	checkpoint := time.NewTicker(time.Duration(p.Request.Config.SaveWorkspaceFrequency) * time.Minute)
	defer checkpoint.Stop()

	errC := make(chan error, 1)
	defer close(errC)

	go func() {
		errC <- cmd.Wait()
	}()

	for {
		select {
		case line := <-cmd.OutputChan:

			// Save any output logging progress to the raw append only log
			f.WriteString(line)

		case <-checkpoint.C:

			for group, artifact := range refresh {
				p.returnOne(group, artifact)
			}

		case err = <-errC:
			return err
		}
	}
}

// fetchAll is used to retrieve from the storage system employed by studioml any and all available
// artifacts and to unpack them into the experiement directory
//
func (p *processor) fetchAll() (err error) {
	// Extract all available artifacts into subdirectories of the main experiment directory.
	//
	// The current convention is that the archives include the directory name under which
	// the files are unpacked in their table of contents
	//
	for group, artifact := range p.Request.Experiment.Artifacts {
		// Process the qualified URI and use just the path for now
		uri, err := url.ParseRequestURI(artifact.Qualified)
		if err != nil {
			return err
		}
		path := strings.TrimLeft(uri.EscapedPath(), "/")
		dest := filepath.Join(p.ExprDir, group)
		if err = os.MkdirAll(dest, 0777); err != nil {
			return err
		}

		var storage runner.Storage
		switch uri.Scheme {
		case "gs":
			storage, err = runner.NewGSstorage(p.Request.Config.Database.ProjectId, artifact.Bucket, true, 15*time.Second)
		case "s3":
			storage, err = runner.NewS3storage(p.Request.Config.Database.ProjectId, uri.Host, artifact.Bucket, true, 15*time.Second)
		default:
			return fmt.Errorf("unknown URI scheme %s passed in studioml request", uri.Scheme)
		}
		if err != nil {
			return err
		}
		if err = storage.Fetch(artifact.Key, true, dest, 5*time.Second); err != nil {
			logger.Info(fmt.Sprintf("data not found for artifact %s using %s due to %s", group, path, err.Error()))
		} else {
			logger.Debug(fmt.Sprintf("extracted %s to %s", path, dest))
		}
		storage.Close()
	}
	return nil
}

// returnOne is used to upload a single artifact to the data store specified by the experimenter
//
func (p *processor) returnOne(group string, artifact runner.Modeldir) (err error) {
	uri, err := url.ParseRequestURI(artifact.Qualified)
	if err != nil {
		return err
	}
	path := strings.TrimLeft(uri.EscapedPath(), "/")

	var storage runner.Storage
	switch uri.Scheme {
	case "gs":
		storage, err = runner.NewGSstorage(p.Request.Config.Database.ProjectId, artifact.Bucket, true, 15*time.Second)
	case "s3":
		storage, err = runner.NewS3storage(p.Request.Config.Database.ProjectId, uri.Host, artifact.Bucket, true, 15*time.Second)
	default:
		return fmt.Errorf("unknown URI scheme %s passed in studioml request", uri.Scheme)
	}

	source := filepath.Join(p.ExprDir, group)
	logger.Debug(fmt.Sprintf("returning %s to %s", source, path))
	if err = storage.Deposit(source, artifact.Key, 5*time.Minute); err != nil {
		logger.Warn(fmt.Sprintf("%s data not uploaded due to %s", group, err.Error()))
	}
	storage.Close()

	return nil
}

// returnAll creates tar archives of the experiments artifacts and then puts them
// back to the studioml shared storage
//
func (p *processor) returnAll() (err error) {

	returned := make([]string, 0, len(p.Request.Experiment.Artifacts))

	for group, artifact := range p.Request.Experiment.Artifacts {
		if !artifact.Mutable {
			if err = p.returnOne(group, artifact); err != nil {
				return err
			}
		}
	}

	if len(returned) != 0 {
		logger.Info(fmt.Sprintf("project %s returning %s", p.Request.Config.Database.ProjectId, strings.Join(returned, ", ")))
	}

	return nil
}

// allocate is used to reserve the resources on the local host needed to handle the entire job as
// a highwater mark.
//
// The returned alloc structure should be used with the deallocate function otherwise resource
// leaks will occur.
//
func (p *processor) allocate() (alloc *runner.Allocated, err error) {

	rqst := runner.AllocRequest{
		Group: p.Group,
	}

	// Before continuing locate GPU resources for the task that has been received
	//
	if rqst.MaxGPUMem, err = runner.ParseBytes(p.Request.Config.Resource.GpuMem); err != nil {
		logger.Debug(fmt.Sprintf("could not handle the gpuMemory value %s due to %v", p.Request.Config.Resource.GpuMem, err))
		// TODO Add an output function here for Issues #4, https://github.com/SentientTechnologies/studio-go-runner/issues/4
		return nil, err
	}

	rqst.MaxGPU = uint(p.Request.Config.Resource.Gpus)

	rqst.MaxCPU = uint(p.Request.Config.Resource.Cpus)
	if rqst.MaxMem, err = humanize.ParseBytes(p.Request.Config.Resource.Ram); err != nil {
		return nil, err
	}
	if rqst.MaxDisk, err = humanize.ParseBytes(p.Request.Config.Resource.Hdd); err != nil {
		return nil, err
	}

	if alloc, err = resources.AllocResources(rqst); err != nil {
		logger.Info(fmt.Sprintf("alloc %s failed due to %v", spew.Sdump(p.Request.Config.Resource), err))
		return nil, err
	}

	logger.Debug(fmt.Sprintf("alloc %s, gave %s", spew.Sdump(rqst), spew.Sdump(*alloc)))

	return alloc, err
}

// deallocate first releases resources and then triggers a ready channel to notify any listener that the
func (p *processor) deallocate(alloc *runner.Allocated) {

	if errs := alloc.Release(); len(errs) != 0 {
		for _, err := range errs {
			logger.Warn(fmt.Sprintf("dealloc %s rejected due to %v", spew.Sdump(*alloc), err))
		}
	} else {
		logger.Debug(fmt.Sprintf("released %s", spew.Sdump(*alloc)))
	}

	// Only wait a second to alter others that the resources have been released
	//
	select {
	case <-time.After(time.Second):
	case p.ready <- true:
	}
}

func reinstateEnv(env []string) {
	os.Clearenv()
	for _, envkv := range env {
		kv := strings.SplitN(envkv, "=", 2)
		if err := os.Setenv(kv[0], kv[1]); err != nil {
			logger.Warn(fmt.Sprintf("could not restore the environment table due %s ", err.Error()))
		}
	}
}

// ProcessMsg is the main function where experiment processing occurs.
//
// This function blocks.
//
func (p *processor) Process(msg *pubsub.Message) (wait *time.Duration, err error) {
	// Store then reload the environment table bracketing the task processing
	environ := os.Environ()
	defer reinstateEnv(environ)

	// Call the allocation function to get access to resources and get back
	// the allocation we recieved
	alloc, err := p.allocate()
	if err != nil {
		msg.Nack()

		err = fmt.Errorf("allocation fail backing off due to %v", err)
		logger.Debug(err.Error())
		return &errBackoff, err
	}

	// Setup a function to release resources that have been allocated
	defer p.deallocate(alloc)

	// Use a panic handler to catch issues related to, or unrelated to the runner
	//
	defer func() {
		recover()
	}()

	// The allocation details are passed in to the runner to allow the
	// resource reservations to become known to the running applications
	if err = p.run(alloc); err != nil {
		msg.Nack()
		return nil, err
	}

	msg.Ack()

	return nil, nil
}

// mkUniqDir will create a working directory for an experiment
// using the file system calls appropriately so as to make sure
// no other instance of the same experiement is using it.  It is
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
func (p *processor) mkUniqDir() (err error) {

	self := uuid.NewV4().String()

	inst := 0
	for {
		// Loop until we fail to find a directory with the prefix
		for {
			p.ExprDir = filepath.Join(p.RootDir, "experiments", p.Request.Experiment.Key+"."+strconv.Itoa(inst))
			if _, err = os.Stat(p.ExprDir); err == nil {
				inst++
				continue
			}
			break
		}

		// Create the next directory in sequence with another directory containing our signature
		if err = os.MkdirAll(filepath.Join(p.ExprDir, self), 0777); err != nil {
			p.ExprDir = ""
			return err
		}

		// After creation check to make sure our signature is the only file there, meaning no other entity
		// used the same experiment and instance
		files, _ := ioutil.ReadDir(p.ExprDir)
		if len(files) != 1 {
			logger.Debug(fmt.Sprintf("looking in what should be a single file inside our experiment and find %s", spew.Sdump(files)))
			// Increment the instance for the next pass
			inst++

			// Backoff for a small amount of time, less than a second then attempt again
			<-time.After(time.Duration(rand.Intn(1000)) * time.Millisecond)
			logger.Debug(fmt.Sprintf("collision during creation of %s with %d files", p.ExprDir, len(files)))
			continue
		}
		p.ExprSubDir = p.Request.Experiment.Key + "." + strconv.Itoa(inst)
		return nil
	}
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
func (p *processor) applyEnv(alloc *runner.Allocated) (err error) {

	// Expand %...% pairs by iterating the env table for the process and explicitly replacing on each line
	re := regexp.MustCompile(`(?U)(?:\%(.*)*\%)+`)
	env := map[string]string{}
	for _, v := range os.Environ() {
		kv := strings.Split(v, "=")
		if len(kv) == 2 {
			env[kv[0]] = kv[1]
		}
	}

	// Environment variables need to be applied here to assist in unpacking S3 files etc
	for k, v := range p.Request.Config.Env {

		for _, match := range re.FindAllString(v, -1) {
			if envV, isPresent := env[match[1:len(match)-1]]; isPresent {
				v = strings.Replace(envV, match, envV, -1)
			}
		}
		// Update the processor env table with the resolved value
		p.Request.Config.Env[k] = v

		if err = os.Setenv(k, v); err != nil {
			return fmt.Errorf("could not change the environment table due %s ", err.Error())
		}
	}
	if err = os.Setenv("AWS_SDK_LOAD_CONFIG", "1"); err != nil {
		return err
	}

	// create the map into which customer environment variables will be added to
	// the experiment script
	//
	p.ExprEnvs = map[string]string{"AWS_SDK_LOAD_CONFIG": "1"}

	// Although we copy the env values to the runners env table through they done get
	// automatically included into the script this is done via the makeScript being given
	// a set of env variables as an array that will be written into the script using the receiever
	// contents.
	//
	if alloc.GPU != nil && len(alloc.GPU.Env) != 0 {
		for k, v := range alloc.GPU.Env {
			if err = os.Setenv(k, v); err != nil {
				return fmt.Errorf("could not change the environment var %s due %s ", k, err.Error())
			} else {
				if *debugOpt {
					logger.Trace(fmt.Sprintf("export %s=%s", k, v))
				}
			}
			p.ExprEnvs[k] = v
		}
	}
	return nil
}

// run is called to execute the work unit
//
func (p *processor) run(alloc *runner.Allocated) (err error) {

	// Generates a working directory if successful and puts the name into the structure for this
	// method
	//
	if err = p.mkUniqDir(); err != nil {
		return err
	}

	if !*debugOpt {
		defer os.RemoveAll(p.ExprDir)
	}

	// Update and apply environment variables for the experiment
	p.applyEnv(alloc)

	if *debugOpt {
		// The following log can expose passwords etc.  As a result we do not allow it unless the debug
		// non production flag is explicitly set
		logger.Trace(fmt.Sprintf("experiment → %s → %s →  %s", p.Request.Experiment, p.ExprDir, spew.Sdump(p.Request)))
	}

	if err = p.fetchAll(); err != nil {
		return err
	}

	script := filepath.Join(p.ExprDir, "workspace", uuid.NewV4().String()+".sh")

	// Now we have the files locally stored we can begin the work
	if err = p.makeScript(script); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	refresh := make(map[string]runner.Modeldir, len(p.Request.Experiment.Artifacts))
	for k, v := range p.Request.Experiment.Artifacts {
		if v.Mutable {
			refresh[k] = v
		}
	}

	if err = p.runScript(context.Background(), script, refresh); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	if err = p.returnAll(); err != nil {
		return err
	}

	return nil
}
