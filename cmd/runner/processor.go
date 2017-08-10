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

	"github.com/mholt/archiver"
	"github.com/ryankurte/go-async-cmd"
	"github.com/satori/go.uuid"

	"github.com/SentientTechnologies/studio-go-runner"
	"github.com/davecgh/go-spew/spew"
)

type processor struct {
	RootDir   string
	ExprDir   string
	Request   *runner.Request // merge these two fields, to avoid some data in fb and some in JSON
	Timeout   time.Duration
	Script    string
	Uploading int64

	storage *runner.Storage
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
// was used by the TFStudio work
func (p *processor) Close() (err error) {
	if p.storage != nil {
		p.storage.Close()
	}

	return os.RemoveAll(p.RootDir)

	return nil
}

func (e *processor) makeScript() (err error) {

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, err := template.New("pythonRunner").Parse(
		`#!/bin/bash
mkdir {{.RootDir}}/blob-cache
mkdir {{.RootDir}}/queue
mkdir {{.RootDir}}/artifact-mappings
mkdir {{.RootDir}}/artifact-mappings/{{.Request.Experiment.Key}}
cd {{.ExprDir}}/workspace
virtualenv .
source bin/activate
pip install {{range .Request.Experiment.Pythonenv}}{{if ne . "studio==0.0"}}{{.}} {{end}}{{end}}
if [ "` + "`" + `echo dist/TFStudio-*.tar.gz` + "`" + `" != "dist/TFStudio-*.tar.gz" ]; then
    pip install dist/TFStudio-*.tar.gz
else
    pip install TFStudio
fi
export TFSTUDIO_EXPERIMENT={{.Request.Experiment.Key}}
export TFSTUDIO_HOME={{.RootDir}}
python {{.Request.Experiment.Filename}} {{range .Request.Experiment.Args}}{{.}} {{end}}
`)

	if err != nil {
		return err
	}

	content := new(bytes.Buffer)
	err = tmpl.Execute(content, e)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(e.Script, content.Bytes(), 0744); err != nil {
		return err
	}
	return nil
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

// uploadArtifact is called as a blocking routine that starts
// a go routine to upload a tar archive of an experiments mutable directory
// and return a channel that can be watched for the result of the upload
//
func (e *processor) uploadArtifact(artifact string, location string) (errC chan error) {
	errC = make(chan error, 1)
	go e.uploader(artifact, location, errC)
	return errC
}

// uploader will rollup the contents of an experiments mutable directory
// and send it to the main firebase storage system blocking until done, and
// when done it will send a response to any callers waiting for the result
// on a channel
//
func (e *processor) uploader(artifact string, location string, errC chan error) {

	// When this processing is complete close the channel, pending sends
	// of real errors are done before this close occurs inside the sendIfErr
	// function
	//
	defer close(errC)
	errSendTimeout := time.Duration(10 * time.Second)

	// Create a working directory that can be dropped along with any working files
	tempDir := filepath.Join(e.ExprDir, uuid.NewV4().String())

	err := os.MkdirAll(tempDir, 0777)
	if sendIfErr(err, errC, errSendTimeout) {
		return
	}
	defer os.RemoveAll(tempDir)

	// Archive into a temporary file all of the contents of the artifacts directory
	archive := filepath.Join(tempDir, artifact+".tgz")

	err = archiver.TarGz.Make(archive, []string{filepath.Join(e.ExprDir, artifact)})
	if sendIfErr(err, errC, errSendTimeout) {
		return
	}

	// Having built an archive for uploading upload it to fb storage
	err = e.storage.Return(archive, location+artifact+".tgz", time.Duration(5*time.Minute))
	if sendIfErr(err, errC, errSendTimeout) {
		return
	}
}

func (e *processor) runScript(ctx context.Context, outLocation string) (err error) {

	cmd := gocmd.Command("/bin/bash", "-c", e.Script)

	cmd.OutputChan = make(chan string, 1024)

	f, err := os.Create(filepath.Join(e.ExprDir, "output", "output"))
	if err != nil {
		return err
	}
	defer f.Close()

	// Start the command handling everything else on channels
	cmd.Start()

	// On a regular basis we will flush the log and compress it for uploading to
	// firebase, use the interval specified in the meta data for the job
	//
	checkpoint := time.NewTicker(time.Duration(e.Request.Config.SaveWorkspaceFrequency) * time.Minute)
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

			<-e.uploadArtifact("output", outLocation)

		case err = <-errC:
			return err
		}
	}
}

func (p *processor) processMsg(msg *pubsub.Message) (err error) {
	p.Request, err = runner.UnmarshalRequest(msg.Data)
	if err != nil {
		logger.Debug("could not unmarshal ", string(msg.Data))
		return err
	}

	p.ExprDir = filepath.Join(p.RootDir, "experiments", p.Request.Experiment.Key)
	if err = os.MkdirAll(p.ExprDir, 0777); err != nil {
		return err
	}

	manifest := map[string]runner.Modeldir{
		"modeldir":  p.Request.Experiment.Artifacts.Modeldir,
		"output":    p.Request.Experiment.Artifacts.Output,
		"tb":        p.Request.Experiment.Artifacts.Tb,
		"workspace": p.Request.Experiment.Artifacts.Workspace,
	}

	p.storage, err = runner.NewStorage(p.Request.Config.Database.ProjectId, p.Request.Config.Database.StorageBucket, true, 15*time.Second)
	if err != nil {
		return err
	}

	_, isPresent := manifest["workspace"]
	if !isPresent {
		return fmt.Errorf("the mandatory workspace archive was not found inside the TFStudio task specification")
	}

	logger.Trace(fmt.Sprintf("experiment → %s → %s →  %s", p.Request.Experiment, p.ExprDir, spew.Sdump(p.Request)))

	// Extract all available artifacts into subdirectories of the main experiment directory.
	//
	// The current convention is that the archives include the directory name under which
	// the files are unpacked in their table of contents
	//
	for group, artifact := range manifest {
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

		if err = p.storage.Fetch(path, true, dest, 5*time.Second); err != nil {
			logger.Info(fmt.Sprintf("data not found for artifact %s using %s due to %s", group, path, err.Error()))
		} else {
			logger.Debug(fmt.Sprintf("extracted %s to %s", path, dest))
		}
	}

	msg.Ack()

	p.Timeout = time.Hour
	p.Script = filepath.Join(p.ExprDir, "workspace", uuid.NewV4().String()+".sh")

	// Now we have the files locally stored we can begin the work
	if err = p.makeScript(); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	uri, err := url.ParseRequestURI(manifest["output"].Qualified)
	if err != nil {
		return err
	}
	outLocation := strings.TrimLeft(uri.EscapedPath(), "/")

	if err = p.runScript(context.Background(), outLocation); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	for group, artifact := range manifest {
		if artifact.Mutable {
			uri, err := url.ParseRequestURI(artifact.Qualified)
			if err != nil {
				return err
			}
			path := strings.TrimLeft(uri.EscapedPath(), "/")
			source := filepath.Join(p.ExprDir, group)

			logger.Info(fmt.Sprintf("returning %s to %s", source, path))
			if err = p.storage.Return(source, path, 5*time.Minute); err != nil {
				logger.Warn(fmt.Sprintf("%s data not uploaded due to %s", group, err.Error()))
			}
		}
	}

	return nil
}
