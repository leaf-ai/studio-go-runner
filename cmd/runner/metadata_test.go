package main

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/go-test/deep"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

func waitForMetaDataRun(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err errors.Error) {
	// Wait for prometheus to show the task as having been ran and completed
	pClient := NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", prometheusPort))

	interval := time.Duration(0)

	// Run around checking the prometheus counters for our experiment seeing when the internal
	// project tracking says everything has completed, only then go out and get the experiment
	// results
	//
	for {
		select {
		case <-time.After(interval):
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
			interval = time.Duration(10 * time.Second)
		}
	}
}

func validateRemoteOutput(ctx context.Context, experiment *ExperData, dir string) (err errors.Error) {
	output := filepath.Join(dir, "output.tar")
	if err = downloadOutput(ctx, experiment, output); err != nil {
		return err
	}

	// Now just unarchive the latest output file for successfully running the python code,
	// to test for its presence and well formed nature but dont use the files for anything
	if errGo := archiver.Tar.Open(output, dir); errGo != nil {
		return errors.Wrap(errGo).With("file", output).With("stack", stack.Trace().TrimRuntime())
	}
	outputDir := path.Join(dir, "output")
	if errGo := os.RemoveAll(outputDir); errGo != nil {
		return errors.Wrap(errGo).With("dir", outputDir).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func checkMDCount(ctx context.Context, experiment *ExperData) (err errors.Error) {

	fCount := 4

	// Query the metadata area on the minio server for our two output files
	names, err := lsMetadata(ctx, experiment)
	if err != nil {
		return err
	}
	if len(names) > fCount {
		return errors.New("too many metadata output logs found").With("expected_count", fCount, "outputs", strings.Join(names, ","), "stack", stack.Trace().TrimRuntime())
	}
	if len(names) < fCount {
		return errors.New("too few metadata output logs found").With("expected_count", fCount, "outputs", strings.Join(names, ","), "stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func validateOutputMultiPass(dir string, ctx context.Context, experiment *ExperData) (err errors.Error) {
	// Get the two expected output logs from the minio server into a working area
	outputDir := filepath.Join(dir, "metadata")
	if errGo := os.MkdirAll(outputDir, 0700); errGo != nil {
		return errors.Wrap(errGo).With("directory", outputDir).With("stack", stack.Trace().TrimRuntime())
	}
	if err = downloadMetadata(ctx, experiment, outputDir); err != nil {
		return err
	}

	files := []string{}
	filepath.Walk(outputDir, func(path string, f os.FileInfo, err error) error {
		if strings.HasPrefix(f.Name(), "output-host-") && strings.HasSuffix(f.Name(), ".log") {
			files = append(files, path)
		}
		return nil
	})
	if len(files) != 2 {
		return errors.New("incorrect number of output meta data files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	// Sort the 2 output files into ascending order which should reflect the wall clock date time order
	// that they were executed in
	sort.Strings(files)
	handles := make([]*os.File, 0, len(files))
	for _, aName := range files {
		f, errGo := os.Open(aName)
		if errGo != nil {
			return errors.Wrap(errGo).With("file", aName).With("stack", stack.Trace().TrimRuntime())
		}
		handles = append(handles, f)
		defer f.Close()
	}

	// This file scan is for a failed run of the experiment
	//
	exitSeen := false
	exitWanted := "+ exit 255"
	s := bufio.NewScanner(handles[0])
	s.Split(bufio.ScanLines)
	for s.Scan() {
		if s.Text() == exitWanted {
			exitSeen = true
			break
		}
	}
	if !exitSeen {
		if err != nil {
			err = errors.New("experiment failure missing").With("exit_output_needed", exitWanted, "stack", stack.Trace().TrimRuntime())
		}
	}

	// Find the json output expected from the python test and also ensure that there
	// was a clean exit for the 2nd file in the s3 side _metadata artifact
	jsonSeen := false
	jsonWanted := `{"experiment": {"name": "Zaphod Beeblebrox"}}`
	exitSeen = false
	exitWanted = "+ exit 0"
	s = bufio.NewScanner(handles[1])
	s.Split(bufio.ScanLines)
	for s.Scan() {
		switch s.Text() {
		case jsonWanted:
			jsonSeen = true
		case exitWanted:
			exitSeen = true
		default:
			continue
		}
		if jsonSeen && exitSeen {
			break
		}
	}
	if errGo := s.Err(); errGo != nil {
		if err != nil {
			err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	if !jsonSeen {
		if err != nil {
			err = errors.New("experiment output missing").With("json_output_needed", jsonWanted, "stack", stack.Trace().TrimRuntime())
		}
	}
	if !exitSeen {
		if err != nil {
			err = errors.New("experiment completion missing").With("exit_output_needed", exitWanted, "stack", stack.Trace().TrimRuntime())
		}
	}

	files = []string{}
	// Now we transition to scraping the json line oriented meta data files
	filepath.Walk(outputDir, func(path string, f os.FileInfo, err error) error {
		if strings.HasPrefix(f.Name(), "scrape-host-") && strings.HasSuffix(f.Name(), ".json") {
			files = append(files, path)
		}
		return nil
	})
	if len(files) != 2 {
		return errors.New("incorrect number of json scrape files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	return err
}

func validateJSonMultiPass(dir string, ctx context.Context, experiment *ExperData) (err errors.Error) {

	// Get the two expected output logs from the minio server into a working area
	outputDir := filepath.Join(dir, "metadata")
	if errGo := os.MkdirAll(outputDir, 0700); errGo != nil {
		return errors.Wrap(errGo).With("directory", outputDir).With("stack", stack.Trace().TrimRuntime())
	}
	if err = downloadMetadata(ctx, experiment, outputDir); err != nil {
		return err
	}

	fInfo := map[string]os.FileInfo{}
	files := []string{}
	filepath.Walk(outputDir, func(path string, f os.FileInfo, err error) error {
		if strings.HasPrefix(f.Name(), "scrape-host-") && strings.HasSuffix(f.Name(), ".json") {
			files = append(files, path)
			fInfo[path] = f
		}
		return nil
	})
	if len(files) != 2 {
		return errors.New("incorrect number of json meta data files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	// Sort the 2 output files into ascending order which should reflect the wall clock date time order
	// that they were executed in
	sort.Strings(files)

	// The first file needs to be empty, for an empty json document to pass

	info, isPresent := fInfo[files[0]]
	if !isPresent {
		return errors.New("file info not found").With("file", files[0]).With("stack", stack.Trace().TrimRuntime())
	}
	if info.Size() != 0 {
		if info.Size() == 3 {
			data, errGo := ioutil.ReadFile(files[0])
			if errGo != nil {
				return errors.Wrap(errGo).With("file", files[0]).With("stack", stack.Trace().TrimRuntime())
			}
			if diff := deep.Equal(data, []byte("{}\n")); diff != nil {
				return errors.New("unexpected empty json file").With("file", files[0], "size", info.Size(), "diff", diff).With("stack", stack.Trace().TrimRuntime())
			}
		} else {
			return errors.New("unexpected zero length file").With("file", files[0], "size", info.Size()).With("stack", stack.Trace().TrimRuntime())
		}
	}

	// Make sure the second file is not empty
	info, isPresent = fInfo[files[1]]
	if !isPresent {
		return errors.New("file info not found").With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
	}
	if info.Size() == 0 {
		return errors.New("unexpected non-zero length file").With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
	}

	// Avoid the first what should be empty file
	f, errGo := os.Open(files[1])
	if errGo != nil {
		return errors.Wrap(errGo).With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()

	// expected := `{"experiment": {"name": "Zaphod Beeblebrox"}}`

	s := bufio.NewScanner(f)
	s.Split(bufio.ScanLines)
	for s.Scan() {
		if s.Text()[0] == '{' {
			return nil
		}
	}
	if err == nil {
		err = errors.New("experiment missing scraped json output").With("expected", "{", "stack", stack.Trace().TrimRuntime())
	}

	return err
}

func validateMultiPassMetaData(ctx context.Context, experiment *ExperData) (err errors.Error) {

	// Should loop until we see the final message saying everything is OK

	// Unpack the output archive within a temporary directory and use it for validation
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	if err = validateRemoteOutput(ctx, experiment, dir); err != nil {
		return err
	}

	if err = checkMDCount(ctx, experiment); err != nil {
		return err
	}

	if err = validateOutputMultiPass(dir, ctx, experiment); err != nil {
		return err
	}

	if err = validateJSonMultiPass(dir, ctx, experiment); err != nil {
		return err
	}
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

	if !*skipCheckK8s {
		if err := runner.IsAliveK8s(); err != nil {
			t.Fatal(err)
		}
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

	if err := runStudioTest(workDir, 0, true, waitForMetaDataRun, validateMultiPassMetaData); err != nil {
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
