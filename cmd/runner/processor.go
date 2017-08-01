package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
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
	return os.RemoveAll(p.dir)
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

	logger.Debug(fmt.Sprintf("experiment → %s → %s", rqst.Experiment, spew.Sdump(rqst)))
	logger.Debug(fmt.Sprintf("experiment → %s → %s", rqst.Experiment, spew.Sdump(manifest)))

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
	defer os.RemoveAll(wrkDir)

	for collection, wrkSpace := range manifest {
		if collection != "output" {
			logger.Info(fmt.Sprintf("extracting %s to %s", wrkSpace, wrkDir))

			err = s.Fetch(wrkSpace, true, wrkDir, 5*time.Second)
			if err != nil {
				logger.Warn(fmt.Sprintf("data not found for type %s", collection))
			}
		}
	}

	msg.Ack()

	// Now we have the files locally stored we can begin the work

	return nil
}
