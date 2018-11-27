package main

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

func waitForMetaDataRun(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err errors.Error) {
	// Wait for prometheus to show the task as having been ran and completed
	pClient := NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", prometheusPort))

	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	// Run around checking the prometheus counters for our experiment seeing when the internal
	// project tracking says everything has completed, only then go out and get the experiment
	// results
	//
	for {
		select {
		case <-tick.C:
			metrics, err := pClient.Fetch("runner_project_")
			if err != nil {
				return err
			}

			runningCnt, finishedCnt, err := projectStats(metrics, qName, queueType, r.Config.Database.ProjectId, r.Experiment.Key)
			if err != nil {
				return err
			}

			// Wait for prometheus to show the task stopped for our specific queue, host, project and experiment ID
			if runningCnt == 0 && finishedCnt == 2 {
				return nil
			}
			logger.Info("stats", "runner", runningCnt, "finished", finishedCnt)
		}
	}
}

func validateMultiPassMetaData(ctx context.Context, experiment *ExperData) (err errors.Error) {

	// Should loop until we see the final message saying everything is OK

	// Unpack the output archive within a temporary directory and use it for validation
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, "output.tar")
	if err = downloadOutput(ctx, experiment, output); err != nil {
		return err
	}

	// Now examine the file for successfully running the python code
	if errGo = archiver.Tar.Open(output, dir); errGo != nil {
		return errors.Wrap(errGo).With("file", output).With("stack", stack.Trace().TrimRuntime())
	}

	outFn := filepath.Join(dir, "output")
	outFile, errGo := os.Open(outFn)
	if errGo != nil {
		return errors.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	supressDump := false
	defer func() {
		if !supressDump {
			io.Copy(os.Stdout, outFile)
		}
		outFile.Close()
	}()

	return nil
}

// TestÄE2EMetadataMultiPassRun is used to exercise an experiment that fails on the first pass and
// stops intentionally and then recovers on the second pass to produce some useful metadata.  The
// test validation checks that the two experiments were run and output, runner and scrape files
// were all generated in the correct manner
//
func TestÄE2EMetadataMultiPassRun(t *testing.T) {
	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	// Navigate to the assets directory being used for this experiment
	workDir, errGo := filepath.Abs(filepath.Join(wd, "..", "..", "assets", "multistep"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	if err := runStudioTest(workDir, 0, waitForMetaDataRun, validateMultiPassMetaData); err != nil {
		t.Fatal(err)
	}

	// Make sure we returned to the directory we expected
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if newWD != wd {
		t.Fatal(errors.New("finished in an unexpected directory").With("expected_dir", wd).With("actual_dir", newWD).With("stack", stack.Trace().TrimRuntime()))
	}
}
