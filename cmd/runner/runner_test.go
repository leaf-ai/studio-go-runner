// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of tests related to starting python based work and
// running it to completion within the server.  Work run is prepackaged within the source
// code repository and orchestrated by the testing within this file.

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/pkg/server"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	minio "github.com/minio/minio-go"

	"github.com/mholt/archiver"
	"github.com/rs/xid"

	"github.com/karlmutch/copy"
)

var (
	// Extracts three floating point values from a tensorflow output line typical for the experiments
	// found within the tf packages.  A typical log line will appear as follows
	// '60000/60000 [==============================] - 1s 23us/step - loss: 0.2432 - acc: 0.9313 - val_loss: 0.2316 - val_acc: 0.9355'
	tfExtract = regexp.MustCompile(`(?mU).*loss:\s([-+]?[0-9]*\.[0-9]*)\s.*acc:\s([-+]?[0-9]*\.[0-9]*)\s.*val_loss:\s([-+]?[0-9]*\.[0-9]*)\s.*val_acc:\s([-+]?[0-9]*\.[0-9]*)$`)
)

func TestATFExtract(t *testing.T) {
	tfResultsExample := `60000/60000 [==============================] - 1s 23us/step - loss: 0.2432 - acc: 0.9313 - val_loss: 0.2316 - val_acc: 0.9355`

	expectedOutput := []string{
		tfResultsExample,
		"0.2432",
		"0.9313",
		"0.2316",
		"0.9355",
	}

	matches := tfExtract.FindAllStringSubmatch(tfResultsExample, -1)
	for i, match := range expectedOutput {
		if matches[0][i] != match {
			t.Fatal(kv.NewError("a tensorflow result not extracted").With("expected", match).With("captured_match", matches[0][i]).With("stack", stack.Trace().TrimRuntime()))
		}
	}
}

func validateTFMinimal(ctx context.Context, experiment *ExperData, rpts []*reports.Report, pythonLogs []string) (err kv.Error) {
	// Unpack the output archive within a temporary directory and use it for validation
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, "output.tar")
	if err = downloadOutput(ctx, experiment, output); err != nil {
		return err
	}

	// Now examine the file for successfully running the python code
	if errGo = archiver.Tar.Open(output, dir); errGo != nil {
		return kv.Wrap(errGo).With("file", output).With("stack", stack.Trace().TrimRuntime())
	}

	outFn := filepath.Join(dir, "output")
	outFile, errGo := os.Open(outFn)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	supressDump := false
	defer func() {
		if !supressDump {
			io.Copy(os.Stdout, outFile)
		}
		outFile.Close()
	}()

	// Typical values for these items inside the TF logging are as follows
	// "loss: 0.2432 - acc: 0.9313 - val_loss: 0.2316 - val_acc: 0.9355"
	acceptableVals := []float64{
		0.35,
		0.85,
		0.35,
		0.85,
	}

	matches := [][]string{}
	scanner := bufio.NewScanner(outFile)
	for scanner.Scan() {
		matched := tfExtract.FindAllStringSubmatch(scanner.Text(), -1)
		if len(matched) != 1 {
			continue
		}
		if len(matched[0]) != 5 {
			continue
		}
		matches = matched
	}
	if errGo = scanner.Err(); errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	if len(matches) != 1 {
		outFile.Seek(0, io.SeekStart)
		io.Copy(os.Stdout, outFile)
		return kv.NewError("unable to find any TF results in the log file").With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	// Although the following values are not using epsilon style float adjustments because
	// the test limits and values are abitrary anyway

	// loss andf accuracy checks against the log data that was extracted using a regular expression
	// and captures
	loss, errGo := strconv.ParseFloat(matches[0][1], 64)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("line", scanner.Text()).With("value", matches[0][1]).With("stack", stack.Trace().TrimRuntime())
	}
	if loss > acceptableVals[1] {
		return kv.NewError("loss is too large").With("file", outFn).With("line", scanner.Text()).With("value", loss).With("ceiling", acceptableVals[1]).With("stack", stack.Trace().TrimRuntime())
	}
	loss, errGo = strconv.ParseFloat(matches[0][3], 64)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("value", matches[0][3]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if loss > acceptableVals[3] {
		return kv.NewError("validation loss is too large").With("file", outFn).With("line", scanner.Text()).With("value", loss).With("ceiling", acceptableVals[3]).With("stack", stack.Trace().TrimRuntime())
	}
	// accuracy checks
	accu, errGo := strconv.ParseFloat(matches[0][2], 64)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("value", matches[0][2]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if accu < acceptableVals[2] {
		return kv.NewError("accuracy is too small").With("file", outFn).With("line", scanner.Text()).With("value", accu).With("ceiling", acceptableVals[2]).With("stack", stack.Trace().TrimRuntime())
	}
	accu, errGo = strconv.ParseFloat(matches[0][4], 64)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("value", matches[0][4]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if accu < acceptableVals[3] {
		return kv.NewError("validation accuracy is too small").With("file", outFn).With("line", scanner.Text()).With("value", accu).With("ceiling", acceptableVals[3]).With("stack", stack.Trace().TrimRuntime())
	}

	logger.Info(matches[0][0], "stack", stack.Trace().TrimRuntime())
	supressDump = true

	return nil
}

func lsMetadata(ctx context.Context, experiment *ExperData) (names []string, err kv.Error) {
	names = []string{}

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return names, kv.Wrap(errGo).With("address", experiment.MinioAddress).With("stack", stack.Trace().TrimRuntime())
	}
	// Create a done channel to control 'ListObjects' go routine.
	doneCh := make(chan struct{})

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	isRecursive := true
	prefix := "metadata/"
	objectCh := mc.ListObjects(experiment.Bucket, prefix, isRecursive, doneCh)
	for object := range objectCh {
		if object.Err != nil {
			return names, kv.Wrap(object.Err).With("address", experiment.MinioAddress).With("stack", stack.Trace().TrimRuntime())
		}
		names = append(names, fmt.Sprint(object.Key))
	}
	return names, nil
}

func downloadMetadata(ctx context.Context, experiment *ExperData, outputDir string) (err kv.Error) {
	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return kv.Wrap(errGo).With("address", experiment.MinioAddress).With("stack", stack.Trace().TrimRuntime())
	}
	// Create a done channel to control 'ListObjects' go routine.
	doneCh := make(chan struct{})

	// Indicate to our routine to exit cleanly upon return.
	defer close(doneCh)

	names := []string{}

	isRecursive := true
	prefix := "metadata/"
	objectCh := mc.ListObjects(experiment.Bucket, prefix, isRecursive, doneCh)
	for object := range objectCh {
		if object.Err != nil {
			return kv.Wrap(object.Err).With("address", experiment.MinioAddress).With("stack", stack.Trace().TrimRuntime())
		}
		names = append(names, filepath.Base(object.Key))
	}

	for _, name := range names {
		key := prefix + name
		object, errGo := mc.GetObject(experiment.Bucket, key, minio.GetObjectOptions{})
		if errGo != nil {
			return kv.Wrap(errGo).With("address", experiment.MinioAddress, "bucket", experiment.Bucket, "name", name).With("stack", stack.Trace().TrimRuntime())
		}
		localName := filepath.Join(outputDir, filepath.Base(name))
		localFile, errGo := os.Create(localName)
		if errGo != nil {
			return kv.Wrap(errGo).With("address", experiment.MinioAddress, "bucket", experiment.Bucket, "key", key, "filename", localName).With("stack", stack.Trace().TrimRuntime())
		}
		if _, errGo = io.Copy(localFile, object); errGo != nil {
			return kv.Wrap(errGo).With("address", experiment.MinioAddress, "bucket", experiment.Bucket, "key", key, "filename", localName).With("stack", stack.Trace().TrimRuntime())
		}
	}
	return nil
}

func downloadOutput(ctx context.Context, experiment *ExperData, output string) (err kv.Error) {

	archive, errGo := os.Create(output)
	if errGo != nil {
		return kv.Wrap(errGo).With("output", output).With("stack", stack.Trace().TrimRuntime())
	}
	defer archive.Close()

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return kv.Wrap(errGo).With("address", experiment.MinioAddress).With("stack", stack.Trace().TrimRuntime())
	}

	exists, errGo := mc.BucketExists(experiment.Bucket)
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", experiment.Bucket, "object", "output.tar", "output", output).With("stack", stack.Trace().TrimRuntime())
	}
	if !exists {
		return kv.NewError("bucket not found").With("bucket", experiment.Bucket).With("stack", stack.Trace().TrimRuntime())
	}

	objects := []minio.ObjectInfo{}
	objectCh := mc.ListObjects("mybucket", "", false, nil)
	for object := range objectCh {
		if object.Err != nil {
			continue
		}
		objects = append(objects, object)
	}

	object, errGo := mc.GetObjectWithContext(ctx, experiment.Bucket, "output.tar", minio.GetObjectOptions{})
	if errGo != nil {
		return kv.Wrap(errGo).With("minio", experiment.MinioAddress, "bucket", experiment.Bucket, "object", "output.tar", "objects", spew.Sdump(objects), "output", output).With("stack", stack.Trace().TrimRuntime())
	}

	if _, errGo = io.Copy(archive, object); errGo != nil {
		return kv.Wrap(errGo).With("minio", experiment.MinioAddress, "bucket", experiment.Bucket, "object", "output.tar", "objects", spew.Sdump(objects), "output", output).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func TestRelocation(t *testing.T) {

	// Keep a record of the directory where we are currently located
	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	// Create a test directory
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer os.RemoveAll(dir)

	func() {
		// Relocate to our new directory and then use the construct of a function
		// to pop back out of the test directory to ensure we are in the right location
		reloc, err := relocateToTemp(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer reloc.Close()
	}()

	// find out where we are and make sure it is where we expect
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if wd != newWD {
		t.Fatal(kv.NewError("relocation could not be reversed").With("origin", wd).With("recovered_to", newWD).With("temp_dir", dir).With("stack", stack.Trace().TrimRuntime()))
	}
}

func TestNewRelocation(t *testing.T) {

	// Keep a record of the directory where we are currently located
	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	// Working directory location that is generated by the functions under test
	tmpDir := ""

	func() {
		// Relocate to a new directory which has had a temporary name generated on
		// out behalf as a working area
		reloc, err := relocateToTransitory()
		if err != nil {
			t.Fatal(err)
		}
		// Make sure we are sitting in another directory at this point and place a test
		// file in it so that later we can check that is got cleared
		tmpDir, errGo = os.Getwd()
		fn := filepath.Join(tmpDir, "EmptyFile")
		fl, errGo := os.Create(fn)
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}
		msg := "test file that should be gathered up and deleted at the end of the Transitory dir testing"
		if _, errGo = fl.WriteString(msg); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime()))
		}
		fl.Close()

		defer reloc.Close()
	}()

	// find out where we are and make sure it is where we expect
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	// Make sure this was not a NOP
	if wd != newWD {
		t.Fatal(kv.NewError("relocation could not be reversed").With("origin", wd).With("recovered_to", newWD).With("temp_dir", tmpDir).With("stack", stack.Trace().TrimRuntime()))
	}

	// Make sure our working directory was cleaned up
	if _, errGo := os.Stat(tmpDir); !os.IsNotExist(errGo) {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
}

// TestÄE2EExperiment is a function used to exercise the core ability of the runner to successfully
// complete a single experiment.  The name of the test uses a Latin A with Diaresis to order this
// test after others that are simpler in nature.
//
// This test take a minute or two but is left to run in the short version of testing because
// it exercises the entire system under test end to end for experiments running in the python
// environment
//
func TestÄE2ECPUExperiment(t *testing.T) {
	// Let the run function load default cases  with 0 GPUs
	opts := E2EExperimentOpts{
		SendReports:   true,
		ListenReports: true,
		Cases:         []E2EExperimentCase{},
	}
	E2EExperimentRun(t, opts)
}

// TestÄE2EGPUExperiment is a rerun of the TestÄE2ECPUExperimen experiment with a GPU
// enabled
//
func TestÄE2EGPUExperiment(t *testing.T) {
	if !*runner.UseGPU {
		logger.Warn("TestÄE2EExperiment not run")
		t.Skip("GPUs disabled for testing")
	}
	opts := E2EExperimentOpts{
		SendReports:   true,
		ListenReports: true,
		Cases: []E2EExperimentCase{
			E2EExperimentCase{
				GPUs:       1,
				useEncrypt: true,
				testAssets: []string{"tf_minimal"},
				Waiter:     waitForRun,
				Validation: validateTFMinimal,
			},
		},
	}
	E2EExperimentRun(t, opts)
}

// TestÄE2EExperimentResponseQ is a rerun of the TestÄE2ECPUExperiment
// for CPUs that uses a python application to watch the response queue
//
func TestÄE2EExperimentPythonResponseQ(t *testing.T) {

	if err := server.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	assetDir, errGo := filepath.Abs(filepath.Join("..", "..", "assets"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	// Kickoff an arbitrary prototypical test case, allow the
	// typical Tensorflow test code to run
	opts := E2EExperimentOpts{
		AssetDir:      assetDir,
		SendReports:   true,
		ListenReports: false,
		PythonReports: true,
		Cases: []E2EExperimentCase{
			E2EExperimentCase{
				GPUs:       0,
				useEncrypt: true,
				testAssets: []string{"tf_minimal"},
				Waiter:     waitForRun,
				Validation: validateTFMinimal,
			}},
	}

	// Start the python listener that runs until the response queue
	// is cleaned up by the test runner then look at its stdout for
	// testing results
	E2EExperimentRun(t, opts)
}

type E2EExperimentCase struct {
	QueueName  string         // The name of the queue that the work should be scheduled on
	GPUs       int            // The number of required GPUs for the experiment
	useEncrypt bool           // Should the request queue be using encryption
	testAssets []string       // The sub tests from the asset directory that need to be included
	Waiter     waitFunc       // Custom wait function for experiment progress monitoring
	Validation validationFunc // Validation function for asserting the results of the test
}

type E2EExperimentOpts struct {
	WorkDir       string              // The working directory where the experiment runtime is expected
	AssetDir      string              // The location of the reference assets directory that contains individual test assets directories
	SendReports   bool                // Should a response queue send report messages
	ListenReports bool                // Use the internal go implementation of a listener
	PythonReports bool                // Use the internal python implementation of a listener
	NoK8sCheck    bool                // Dont validate the presence of Kubernetes in this test
	Cases         []E2EExperimentCase // Per experiment test parameters
}

func E2EExperimentRun(t *testing.T, opts E2EExperimentOpts) {

	if err := server.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	} else {
		opts.NoK8sCheck = true
	}

	gpuCount := runner.GPUCount()

	assetDir := opts.AssetDir
	if len(assetDir) == 0 {
		dir, errGo := filepath.Abs(filepath.Join("..", "..", "assets"))
		if errGo != nil {
			t.Fatal(errGo)
		}
		assetDir = dir
	}

	if len(opts.Cases) == 0 {
		opts.Cases = append(opts.Cases,
			E2EExperimentCase{
				GPUs:       0,
				useEncrypt: false,
				testAssets: []string{"tf_minimal"},
				Waiter:     waitForRun,
				Validation: validateTFMinimal,
			})
		opts.Cases = append(opts.Cases,
			E2EExperimentCase{
				GPUs:       0,
				useEncrypt: true,
				testAssets: []string{"tf_minimal"},
				Waiter:     waitForRun,
				Validation: validateTFMinimal,
			})
	}

	for _, aCase := range opts.Cases {
		// Create a working dir
		if len(opts.WorkDir) == 0 {
			working, errGo := ioutil.TempDir("", "end-to-end")
			if errGo != nil {
				t.Fatal(errGo)
			}
			// Cleanup the test by-products when finished
			defer os.RemoveAll(working)
			opts.WorkDir = working
		}

		if aCase.GPUs > gpuCount {
			t.Skipf("insufficient GPUs %d, needed %d", gpuCount, aCase.GPUs)
		}

		// Copy the contents of the assets directories into the working dir
		if len(aCase.testAssets) != 0 {
			for _, dir := range aCase.testAssets {
				// Copy the standard minimal tensorflow test into a working directory
				if errGo := copy.Copy(filepath.Join(assetDir, dir), opts.WorkDir); errGo != nil {
					t.Fatal(errGo)
				}
			}
		} else {
			if errGo := copy.Copy(filepath.Join(assetDir, "tf_minimal"), opts.WorkDir); errGo != nil {
				t.Fatal(errGo)
			}
		}

		wd, errGo := os.Getwd()
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}

		runOpts := studioRunOptions{
			WorkDir:       opts.WorkDir,
			AssetDir:      assetDir,
			QueueName:     aCase.QueueName,
			GPUs:          aCase.GPUs,
			NoK8sCheck:    opts.NoK8sCheck,
			UseEncryption: aCase.useEncrypt,
			SendReports:   opts.SendReports,
			ListenReports: opts.ListenReports,
			PythonReports: opts.PythonReports,
			Waiter:        aCase.Waiter,
			Validation:    aCase.Validation,
		}

		// The normal timeout is 30 minutes so we preempt that with our own check
		timeout, timeoutCancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer timeoutCancel()

		if err := studioRun(timeout, runOpts); err != nil {
			t.Fatal(err)
		}

		// Make sure we returned to the directory we expected
		newWD, errGo := os.Getwd()
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}
		if newWD != wd {
			t.Fatal(kv.NewError("finished in an unexpected directory").With("expected_dir", wd).With("actual_dir", newWD).With("stack", stack.Trace().TrimRuntime()))
		}
	}
}

func validatePytorchMultiGPU(ctx context.Context, experiment *ExperData, rpts []*reports.Report, pythonLogs []string) (err kv.Error) {
	// Unpack the output archive within a temporary directory and use it for validation
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, "output.tar")
	if err = downloadOutput(ctx, experiment, output); err != nil {
		return err
	}

	// Now examine the file for successfully running the python code
	if errGo = archiver.Tar.Open(output, dir); errGo != nil {
		return kv.Wrap(errGo).With("file", output).With("stack", stack.Trace().TrimRuntime())
	}

	outFn := filepath.Join(dir, "output")
	outFile, errGo := os.Open(outFn)
	if errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	supressDump := false
	defer func() {
		if !supressDump {
			io.Copy(os.Stdout, outFile)
		}
		outFile.Close()
	}()

	validateString := fmt.Sprintf("(\"Let's use\", %dL, 'GPUs!')", len(experiment.GPUs))
	err = kv.NewError("multiple gpu logging not found").With("log", validateString).With("stack", stack.Trace().TrimRuntime())

	scanner := bufio.NewScanner(outFile)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), validateString) {
			supressDump = true
			err = nil
			break
		}
	}
	if errGo = scanner.Err(); errGo != nil {
		return kv.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	return err
}

// TestÄE2EPytorchMGPURun is a function used to exercise the multi GPU ability of the runner to successfully
// complete a single pytorch multi GPU experiment.  The name of the test uses a Latin A with Diaresis to order this
// test after others that are simpler in nature.
//
// This test take a minute or two but is left to run in the short version of testing because
// it exercises the entire system under test end to end for experiments running in the python
// environment
//
func TestÄE2EPytorchMGPURun(t *testing.T) {

	if err := server.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if !*runner.UseGPU {
		logger.Warn("TestÄE2EPytorchMGPURun not run")
		t.Skip("GPUs disabled for testing")
	}

	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	gpusNeeded := 2
	gpuCount := runner.GPUCount()
	if gpusNeeded > gpuCount {
		t.Skipf("insufficient GPUs %d, needed %d", gpuCount, gpusNeeded)
	}

	// Navigate to the assets directory being used for this experiment
	workDir, errGo := filepath.Abs(filepath.Join(wd, "..", "..", "assets", "pytorch_mgpu"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	opts := studioRunOptions{
		WorkDir:       workDir,
		GPUs:          2,
		NoK8sCheck:    false,
		UseEncryption: false,
		SendReports:   false,
		ListenReports: false,
		Waiter:        waitForRun,
		Validation:    validatePytorchMultiGPU,
	}

	if err := studioRun(context.Background(), opts); err != nil {
		t.Fatal(err)
	}

	// Make sure we returned to the directory we expected
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if newWD != wd {
		t.Fatal(kv.NewError("finished in an unexpected directory").With("expected_dir", wd).With("actual_dir", newWD).With("stack", stack.Trace().TrimRuntime()))
	}
}
