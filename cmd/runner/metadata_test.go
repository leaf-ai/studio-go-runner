// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	"google.golang.org/protobuf/encoding/protojson"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

func waitForMetaDataRun(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err kv.Error) {
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

			logger.Info("stats", "runner", runningCnt, "finished", finishedCnt)
			// Wait for prometheus to show the task stopped for our specific queue, host, project and experiment ID
			if runningCnt == 0 && finishedCnt >= 2 {
				return nil
			}
			interval = time.Duration(10 * time.Second)
		}
	}
}

func validateRemoteOutput(ctx context.Context, experiment *ExperData, dir string) (err kv.Error) {
	output := filepath.Join(dir, "output.tar")
	if err = downloadOutput(ctx, experiment, output); err != nil {
		return err
	}

	// Now just unarchive the latest output file for successfully running the python code,
	// to test for its presence and well formed nature but dont use the files for anything
	if errGo := archiver.Tar.Open(output, dir); errGo != nil {
		return kv.Wrap(errGo).With("file", output).With("stack", stack.Trace().TrimRuntime())
	}
	outputDir := path.Join(dir, "output")
	if errGo := os.RemoveAll(outputDir); errGo != nil {
		return kv.Wrap(errGo).With("dir", outputDir).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func checkMDCount(ctx context.Context, experiment *ExperData) (err kv.Error) {

	fCount := 4

	// Query the metadata area on the minio server for our two output files
	names, err := lsMetadata(ctx, experiment)
	if err != nil {
		return err
	}
	if len(names) > fCount {
		return kv.NewError("too many metadata files found").With("expected_count", fCount, "actual_count", len(names), "outputs", strings.Join(names, ","), "stack", stack.Trace().TrimRuntime())
	}
	if len(names) < fCount {
		return kv.NewError("too few metadata files found").With("expected_count", fCount, "actual_count", len(names), "outputs", strings.Join(names, ","), "stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func validateOutputMultiPass(ctx context.Context, dir string, experiment *ExperData) (err kv.Error) {
	// Get the two expected output logs from the minio server into a working area
	outputDir := filepath.Join(dir, "metadata")
	if errGo := os.MkdirAll(outputDir, 0700); errGo != nil {
		return kv.Wrap(errGo).With("directory", outputDir).With("stack", stack.Trace().TrimRuntime())
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
		return kv.NewError("incorrect number of output meta data files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	// Sort the 2 output files into ascending order which should reflect the wall clock date time order
	// that they were executed in
	sort.Strings(files)
	handles := make([]*os.File, 0, len(files))
	for _, aName := range files {
		f, errGo := os.Open(aName)
		if errGo != nil {
			return kv.Wrap(errGo).With("file", aName).With("stack", stack.Trace().TrimRuntime())
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
			err = kv.NewError("experiment failure missing").With("exit_output_needed", exitWanted, "stack", stack.Trace().TrimRuntime())
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
			err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
	if !jsonSeen {
		if err != nil {
			err = kv.NewError("experiment output missing").With("json_output_needed", jsonWanted, "stack", stack.Trace().TrimRuntime())
		}
	}
	if !exitSeen {
		if err != nil {
			err = kv.NewError("experiment completion missing").With("exit_output_needed", exitWanted, "stack", stack.Trace().TrimRuntime())
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
		return kv.NewError("incorrect number of json scrape files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	return err
}

func validateJSonMultiPass(ctx context.Context, dir string, experiment *ExperData, reportingErr *kv.Error) (err kv.Error) {

	// Get the two expected output logs from the minio server into a working area
	outputDir := filepath.Join(dir, "metadata")
	if errGo := os.MkdirAll(outputDir, 0700); errGo != nil {
		return kv.Wrap(errGo).With("directory", outputDir).With("stack", stack.Trace().TrimRuntime())
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
		return kv.NewError("incorrect number of json meta data files found").With("files", strings.Join(files, ","), "stack", stack.Trace().TrimRuntime())
	}

	// Sort the 2 output files into ascending order which should reflect the wall clock date time order
	// that they were executed in
	sort.Strings(files)

	// The first file needs to be empty, for an empty json document to pass

	info, isPresent := fInfo[files[0]]
	if !isPresent {
		return kv.NewError("file info not found").With("file", files[0]).With("stack", stack.Trace().TrimRuntime())
	}
	if info.Size() != 0 {
		// Mask out anything other than the experiment section
		type experiment struct {
			Experiment map[string]interface{} `json:"experiment"`
		}
		exp := &experiment{
			Experiment: map[string]interface{}{},
		}
		data, errGo := ioutil.ReadFile(files[0])
		if errGo != nil {
			return kv.Wrap(errGo).With("file", files[0]).With("stack", stack.Trace().TrimRuntime())
		}
		errGo = json.Unmarshal(data, &exp)
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		if len(exp.Experiment) != 0 {
			return kv.NewError("unexpected experiment section in json file").With("file", files[0], "size", info.Size(), "data", data).With("stack", stack.Trace().TrimRuntime())
		}
	}

	// Make sure the second file is not empty
	info, isPresent = fInfo[files[1]]
	if !isPresent {
		return kv.NewError("file info not found").With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
	}
	if info.Size() == 0 {
		return kv.NewError("unexpected non-zero length file").With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
	}

	// Avoid the first what should be empty file
	f, errGo := os.Open(files[1])
	if errGo != nil {
		return kv.Wrap(errGo).With("file", files[1]).With("stack", stack.Trace().TrimRuntime())
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
		err = kv.NewError("experiment missing scraped json output").With("expected", "{", "stack", stack.Trace().TrimRuntime())
	}

	return err
}

func validateMultiPassMetaData(ctx context.Context, experiment *ExperData, rpts []*runnerReports.Report) (err kv.Error) {

	// Should loop until we see the final message saying everything is OK

	// Unpack the output archive within a temporary directory and use it for validation
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if !*debugOpt {
		defer os.RemoveAll(dir)
	}

	if err = validateRemoteOutput(ctx, experiment, dir); err != nil {
		return err
	}

	if errCount := checkMDCount(ctx, experiment); errCount != nil {
		// Pull the metadata down and dump it to find out the cause
		logger.Warn("failed check of metadata", "error", errCount)

		outputDir, errGo := ioutil.TempDir("", xid.New().String())
		if errGo != nil {
			logger.Warn("failed to download metadata", "error", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		if err := downloadMetadata(ctx, experiment, outputDir); err != nil {
			logger.Warn("failed to download metadata", "error", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
			return err
		}
		_ = filepath.Walk(outputDir, func(path string, info os.FileInfo, errGo error) error {
			if info.IsDir() {
				return nil
			}
			f, errGo := os.Open(path)
			if errGo != nil {
				logger.Warn("failed to fetch metadata", "error", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
				return errGo
			}
			defer f.Close()
			if _, errGo = io.Copy(os.Stdout, f); errGo != nil {
				logger.Warn("failed to read metadata", "error", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
				return errGo
			}
			return nil
		})
		return errCount
	}

	if err = validateOutputMultiPass(ctx, dir, experiment); err != nil {
		return err
	}

	if err = validateJSonMultiPass(ctx, dir, experiment, nil); err != nil {
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

	if err := runner.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if !*skipCheckK8s {
		if err := runner.IsAliveK8s(); err != nil {
			t.Fatal(err)
		}
	}

	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	assetDir, errGo := filepath.Abs(filepath.Join("..", "..", "assets"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	opts := E2EExperimentOpts{
		AssetDir:      assetDir,
		NoK8sCheck:    true,
		SendReports:   true,
		ListenReports: true,
		PythonReports: false,
		Cases: []E2EExperimentCase{
			E2EExperimentCase{
				GPUs:       0,
				useEncrypt: false,
				testAssets: []string{"multistep"},
				Waiter:     waitForMetaDataRun,
				Validation: validateResponseQ,
			},
		}}

	E2EExperimentRun(t, opts)

	// Make sure we returned to the directory we expected
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if newWD != wd {
		t.Fatal(kv.NewError("finished in an unexpected directory").With("expected_dir", wd).With("actual_dir", newWD).With("stack", stack.Trace().TrimRuntime()))
	}
}

func validateResponseQ(ctx context.Context, experiment *ExperData, rpts []*runnerReports.Report, sidecarLogs []string) (err kv.Error) {
	if err = validateMultiPassMetaData(ctx, experiment, rpts); err != nil {
		return err
	}
	// Continue checking into the reports output
	outputs := rpts[len(rpts)-20:]
	if len(outputs) < 10 {
		return kv.NewError("insufficient report messages").With("stack", stack.Trace().TrimRuntime())
	}

	logger.Debug(spew.Sdump(outputs))
	return nil
}

// TestÄE2EPythonResponsesMultiPassRun is used to exercise an experiment that fails on the first pass and
// stops intentionally and then recovers on the second pass to produce some useful metadata.  The
// test validation checks that the two experiment attempts were run and output any response queue
// events in the correct manner
//
func TestÄE2EPythonResponsesMultiPassRun(t *testing.T) {

	if err := runner.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if !*skipCheckK8s {
		if err := runner.IsAliveK8s(); err != nil {
			t.Fatal(err)
		}
	}

	assetDir, errGo := filepath.Abs(filepath.Join("..", "..", "assets"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	opts := E2EExperimentOpts{
		AssetDir:      assetDir,
		NoK8sCheck:    true,
		SendReports:   true,
		ListenReports: false,
		PythonReports: true,
		Cases: []E2EExperimentCase{
			E2EExperimentCase{
				GPUs:       0,
				useEncrypt: false,
				testAssets: []string{"multistep"},
				Waiter:     waitForMetaDataRun,
				Validation: validateMultiResponseQ,
			},
		}}

	E2EExperimentRun(t, opts)
}

func validateMultiResponseQ(ctx context.Context, experiment *ExperData, rpts []*runnerReports.Report, sidecarLogs []string) (err kv.Error) {

	restartsCnt := 0
	parseFailures := 0

	err = kv.NewError("Progress message finished missing").With("stack", stack.Trace().TrimRuntime())

	times := make([]int64, 0, len(rpts))
	events := make(map[int64]*runnerReports.Progress, len(rpts))
	for _, aLine := range sidecarLogs {
		if len(aLine) == 0 {
			continue
		}
		report := &runnerReports.Report{}
		if errGo := protojson.Unmarshal([]byte(aLine), report); errGo != nil {
			parseFailures += 1
			continue
		}
		switch report.Payload.(type) {
		case *runnerReports.Report_Progress:
			events[report.GetProgress().GetTime().Seconds] = report.GetProgress()
			times = append(times, report.GetProgress().GetTime().Seconds)
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })

	for _, evtTime := range times {
		progress, isPresent := events[evtTime]
		if !isPresent {
			continue
		}

		logger.Debug(spew.Sdump(progress))
		switch progress.GetState() {
		case runnerReports.TaskState_Started:
			restartsCnt += 1
		case runnerReports.TaskState_Success:
			// When we see two starts for the experiment and then success we know the experiment
			// was recovered which is the use case we are testing for
			if restartsCnt == 2 {
				err = nil
			}
		case runnerReports.TaskState_Failed:
			err = kv.NewError("Progress message finished missing").With("stack", stack.Trace().TrimRuntime())
			logger.Debug(spew.Sdump(progress))
		}
	}
	if parseFailures != 0 {
		logger.Debug(fmt.Sprintf("report record payload had %d failures", parseFailures))
	}

	if err = validateMultiPassMetaData(ctx, experiment, rpts); err != nil {
		return err
	}

	return nil
}
