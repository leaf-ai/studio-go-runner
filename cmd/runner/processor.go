package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"cloud.google.com/go/pubsub"

	"github.com/ryankurte/go-async-cmd"
	"github.com/satori/go.uuid"

	"github.com/SentientTechnologies/studio-go-runner"
	"github.com/davecgh/go-spew/spew"
)

type processor struct {
	RootDir string          `json:"root_dir"`
	ExprDir string          `json:"expr_dir"`
	Request *runner.Request `json:"request"` // merge these two fields, to avoid split data in a DB and some in JSON
}

// newProcessor will create a new working directory and wire
// up a connection to firebase to retrieve meta data that is not in
// the original JSON request received using googles pubsub
//
func newProcessor(projectID string) (p *processor, err error) {

	p = &processor{}

	// Create a test file for use by the data server emulation
	// Get a location for running the test
	//
	p.RootDir, err = ioutil.TempDir("", uuid.NewV4().String())
	if err != nil {
		return nil, err
	}

	return p, nil
}

// Close will release all resources and clean up the work directory that
// was used by the studioml work
//
func (p *processor) Close() (err error) {
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
mkdir {{.RootDir}}/blob-cache
mkdir {{.RootDir}}/queue
mkdir {{.RootDir}}/artifact-mappings
mkdir {{.RootDir}}/artifact-mappings/{{.Request.Experiment.Key}}
cd {{.ExprDir}}/workspace
virtualenv .
source bin/activate
pip install {{range .Request.Experiment.Pythonenv}}{{if ne . "studio==0.0"}}{{.}} {{end}}{{end}}
if [ "` + "`" + `echo dist/studioml-*.tar.gz` + "`" + `" != "dist/studioml-*.tar.gz" ]; then
    pip install dist/studioml-*.tar.gz
else
    pip install studioml
fi
export STUDIOML_EXPERIMENT={{.Request.Experiment.Key}}
export STUDIOML_HOME={{.RootDir}}
python {{.Request.Experiment.Filename}} {{range .Request.Experiment.Args}}{{.}} {{end}}
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

// sendIfErr will attempt to send an error to anyone watching and timeout if
// no callers are waiting for the error result
//
func sendIfErr(err error, errC chan error, timeout time.Duration) bool {
	if err == nil {
		return false
	}
	select {
	case errC <- err:
	case <-time.After(timeout):
	}
	return false
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

// makeManifest produces a summary of the artifacts and their descriptions for use
// by the processor code
//
func (p *processor) makeManifest() (manifest map[string]runner.Modeldir) {
	return map[string]runner.Modeldir{
		"modeldir":  p.Request.Experiment.Artifacts.Modeldir,
		"output":    p.Request.Experiment.Artifacts.Output,
		"tb":        p.Request.Experiment.Artifacts.Tb,
		"workspace": p.Request.Experiment.Artifacts.Workspace,
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
	for group, artifact := range p.makeManifest() {
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
	// Dont return data is not marked as being mutable
	if !artifact.Mutable {
		return nil
	}

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
	logger.Info(fmt.Sprintf("returning %s to %s", source, path))
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

	for group, artifact := range p.makeManifest() {
		if err = p.returnOne(group, artifact); err != nil {
			return err
		}
	}

	return nil
}

// ProcessMsg is the main function where experiment processing occurs
//
func (p *processor) ProcessMsg(msg *pubsub.Message) (err error) {
	// Store then reload the environment table bracketing the task processing
	environ := os.Environ()
	defer func() {
		os.Clearenv()
		for _, envkv := range environ {
			kv := strings.SplitN(envkv, "=", 2)
			if err := os.Setenv(kv[0], kv[1]); err != nil {
				logger.Warn("could not restore the environment table due %s ", err.Error())
			}
		}
	}()

	p.Request, err = runner.UnmarshalRequest(msg.Data)
	if err != nil {
		logger.Debug("could not unmarshal ", string(msg.Data))
		return err
	}

	p.ExprDir = filepath.Join(p.RootDir, "experiments", p.Request.Experiment.Key)
	if err = os.MkdirAll(p.ExprDir, 0777); err != nil {
		return err
	}

	if err != nil {
		return err
	}

	// Environment variables need to be applied here to assist in unpacking S3 files etc
	for k, v := range p.Request.Config.Env {
		if err := os.Setenv(k, v); err != nil {
			return fmt.Errorf("could not change the environment table due %s ", err.Error())
		}
	}
	if err = os.Setenv("AWS_SDK_LOAD_CONFIG", "1"); err != nil {
		return err
	}

	logger.Trace(fmt.Sprintf("experiment → %s → %s →  %s", p.Request.Experiment, p.ExprDir, spew.Sdump(p.Request)))

	if err = p.fetchAll(); err != nil {
		return err
	}

	script := filepath.Join(p.ExprDir, "workspace", uuid.NewV4().String()+".sh")

	// Now we have the files locally stored we can begin the work
	if err = p.makeScript(script); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	refresh := map[string]runner.Modeldir{"output": p.Request.Experiment.Artifacts.Output}

	if err = p.runScript(context.Background(), script, refresh); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	if err = p.returnAll(); err != nil {
		return err
	}

	msg.Ack()

	return nil
}
