package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"cloud.google.com/go/pubsub"

	"github.com/satori/go.uuid"

	"github.com/davecgh/go-spew/spew"
	"github.com/karlmutch/studio-go-runner"
)

type processor struct {
	// dir is a qwork directory for the runner to place downloaded files etc
	// into
	//
	dir string

	// fb contains a reference for the Firebase instance that runners still rely upon, the
	// FB implementation will be removed as the work messages are upgraded and improved
	//
	fb *runner.FirebaseDB
}

func newProcessor(projectID string) (p *processor, err error) {

	p = &processor{}

	p.fb, err = runner.NewDatabase(projectID)
	if err != nil {
		return nil, err
	}

	// Create a test file for use by the data server emulation
	// Get a location for running the test
	//
	p.dir, err = ioutil.TempDir("", uuid.NewV4().String())
	if err != nil {
		return nil, err
	}

	return p, nil
}

func (p *processor) Close() (err error) {
	// return os.RemoveAll(p.dir)
	return nil
}

func (p *processor) doWork(workDir string, experiment string) (err error) {
	metadata, err := p.fb.GetExperiment(experiment)
	if err != nil {
		return err
	}

	type Vals struct {
		PWD      string
		MetaData *runner.TFSMetaData
	}
	vals := Vals{
		PWD:      workDir,
		MetaData: metadata,
	}

	// Create a shell script that will do everything needed to run
	// the python environment in a virtual env
	tmpl, err := template.New("pythonRunner").Parse(
		`#!/bin/bash
virtualenv {{.PWD}}
source bin/activate
pip install {{range .MetaData.Pythonenv}}{{if ne . "tfstudio==0.0"}}{{.}} {{end}}{{end}}
python {{.MetaData.Filename}} {{range .MetaData.Args}}{{.}} {{end}}
`)
	if err != nil {
		return err
	}

	script := new(bytes.Buffer)
	err = tmpl.Execute(script, vals)
	if err != nil {
		return err
	}

	if err = ioutil.WriteFile(filepath.Join(workDir, uuid.NewV4().String()+".sh"), script.Bytes(), 0744); err != nil {
		return err
	}
	return nil
}

func (p *processor) processMsg(msg *pubsub.Message) (err error) {
	rqst, err := runner.UnmarshalRequest(msg.Data)
	if err != nil {
		return err
	}

	manifest, err := p.fb.GetManifest(rqst.Experiment)
	if err != nil {
		return err
	}

	s, err := runner.NewStorage(rqst.Config.DB.ProjectId, rqst.Config.DB.StorageBucket, true, 15*time.Second)
	if err != nil {
		return err
	}
	defer s.Close()

	_, isPresent := manifest["workspace"]
	if !isPresent {
		return fmt.Errorf("the mandatory workspace archive was not found inside the TFStudio task specification")
	}

	wrkDir := filepath.Join(p.dir, uuid.NewV4().String())
	if err = os.MkdirAll(wrkDir, 0777); err != nil {
		return err
	}

	logger.Debug(fmt.Sprintf("experiment → %s → %s →  %s", rqst.Experiment, wrkDir, spew.Sdump(rqst)))

	for collection, wrkSpace := range manifest {
		if collection != "output" {

			err = s.Fetch(wrkSpace, true, wrkDir, 5*time.Second)
			if err != nil {
				logger.Warn(fmt.Sprintf("data not found for type %s", collection))
			} else {
				logger.Debug(fmt.Sprintf("extracted %s to %s", wrkSpace, wrkDir))
			}
		}
	}

	msg.Ack()

	// Now we have the files locally stored we can begin the work
	if err = p.doWork(wrkDir, rqst.Experiment); err != nil {
		// TODO: We could push work back onto the queue at this point if needed
		return err
	}

	return nil
}
