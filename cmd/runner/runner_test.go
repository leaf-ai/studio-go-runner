package main

// This file contains the implementation of tests related to starting python based work and
// running it to completion within the server.  Work run is prepackaged within the source
// code repository and orchestrated by the testing within this file.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	minio "github.com/minio/minio-go"
	"github.com/streadway/amqp"

	"github.com/mholt/archiver"
	model "github.com/prometheus/client_model/go"
	"github.com/rs/xid"
)

var (
	// Extracts three floating point values from a tensorflow output line typical for the experiments
	// found within the tf packages.  A typical log line will appear as follows
	// '60000/60000 [==============================] - 1s 23us/step - loss: 0.2432 - acc: 0.9313 - val_loss: 0.2316 - val_acc: 0.9355'
	tfExtract = regexp.MustCompile(`(?mU).*loss:\s([-+]?[0-9]*\.[0-9]*)\s.*acc:\s([-+]?[0-9]*\.[0-9]*)\s.*val_loss:\s([-+]?[0-9]*\.[0-9]*)\s.*val_acc:\s([-+]?[0-9]*\.[0-9]*)$`)
)

func TestATFExtractilargeon(t *testing.T) {
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
			t.Fatal(errors.New("a tensorflow result not extracted").With("expected", match).With("captured_match", matches[0][i]).With("stack", stack.Trace().TrimRuntime()))
		}
	}
}

type ExperData struct {
	RabbitMQUser     string
	RabbitMQPassword string
	Bucket           string
	MinioAddress     string
	MinioUser        string
	MinioPassword    string
}

// downloadFile will download a url to a local file using streaming.
//
func downloadFile(fn string, download string) (err errors.Error) {

	// Create the file
	out, errGo := os.Create(fn)
	if errGo != nil {
		return errors.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer out.Close()

	// Get the data
	resp, errGo := http.Get(download)
	if errGo != nil {
		return errors.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	// Write the body to file
	_, errGo = io.Copy(out, resp.Body)
	if errGo != nil {
		return errors.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func downloadRMQCli(fn string) (err errors.Error) {
	if err = downloadFile(fn, os.ExpandEnv("http://${RABBITMQ_SERVICE_SERVICE_HOST}:${RABBITMQ_SERVICE_SERVICE_PORT_RMQ_ADMIN}/cli/rabbitmqadmin")); err != nil {
		return err
	}
	// Having downloaded the administration CLI tool set it to be executable
	if errGo := os.Chmod(fn, 0777); errGo != nil {
		return errors.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// setupRMQ will download the rabbitMQ administration tool from the k8s deployed rabbitMQ
// server and place it into the project bin directory setting it to executable in order
// that diagnostic commands can be run using the shell
//
func setupRMQAdmin(t *testing.T) (err errors.Error) {
	rmqAdmin := path.Join("/project", "bin")
	fi, errGo := os.Stat(rmqAdmin)
	if errGo != nil {
		return errors.Wrap(errGo).With("dir", rmqAdmin).With("stack", stack.Trace().TrimRuntime())
	}
	if !fi.IsDir() {
		return errors.New("specified directory is not actually a directory").With("dir", rmqAdmin).With("stack", stack.Trace().TrimRuntime())
	}

	// Look for the rabbitMQ Server and download the command line tools for use
	// in diagnosing issues, and do this before changing into the test directorya
	rmqAdmin = filepath.Join(rmqAdmin, "rabbitmqadmin")
	if err = downloadRMQCli(rmqAdmin); err != nil {
		t.Fatal(err)
	}
	return nil
}

func collectUploadFiles(dir string) (files []string, err errors.Error) {

	errGo := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			files = append(files, path)
			return nil
		})

	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	sort.Strings(files)

	return files, nil
}

func uploadWorkspace(experiment *ExperData) (err errors.Error) {

	wd, _ := os.Getwd()
	logger.Info(wd, "experiment", fmt.Sprint(*experiment))

	dir := "."
	files, err := collectUploadFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no files found").With("directory", dir).With("stack", stack.Trace().TrimRuntime())
	}

	// Pack the files needed into an archive within a temporary directory
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	archiveName := filepath.Join(dir, "workspace.tar")

	if errGo = archiver.Tar.Make(archiveName, files); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	archive, errGo := os.Open(archiveName)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer archive.Close()

	fileStat, errGo := archive.Stat()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create the bucket that will be used by the experiment, and then place the workspace into it
	if errGo = mc.MakeBucket(experiment.Bucket, ""); errGo != nil {
		switch minio.ToErrorResponse(errGo).Code {
		case "BucketAlreadyExists":
		case "BucketAlreadyOwnedByYou":
		default:
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	_, errGo = mc.PutObject(experiment.Bucket, "workspace.tar", archive, fileStat.Size(),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func validateExperiment(ctx context.Context, experiment *ExperData) (err errors.Error) {
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
		return errors.Wrap(errGo).With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	if len(matches) != 1 {
		return errors.New("unable to find any TF results in the log file").With("file", outFn).With("stack", stack.Trace().TrimRuntime())
	}

	// Although the following values are not using epsilon style float adjustments because
	// the test limits and values are abitrary anyway

	// loss andf accuracy checks against the log data that was extracted using a regular expression
	// and captures
	loss, errGo := strconv.ParseFloat(matches[0][1], 64)
	if errGo != nil {
		return errors.Wrap(errGo).With("file", outFn).With("line", scanner.Text()).With("value", matches[0][1]).With("stack", stack.Trace().TrimRuntime())
	}
	if loss > acceptableVals[1] {
		return errors.New("loss is too large").With("file", outFn).With("line", scanner.Text()).With("value", loss).With("ceiling", acceptableVals[1]).With("stack", stack.Trace().TrimRuntime())
	}
	loss, errGo = strconv.ParseFloat(matches[0][3], 64)
	if errGo != nil {
		return errors.Wrap(errGo).With("file", outFn).With("value", matches[0][3]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if loss > acceptableVals[3] {
		return errors.New("validation loss is too large").With("file", outFn).With("line", scanner.Text()).With("value", loss).With("ceiling", acceptableVals[3]).With("stack", stack.Trace().TrimRuntime())
	}
	// accuracy checks
	accu, errGo := strconv.ParseFloat(matches[0][2], 64)
	if errGo != nil {
		return errors.Wrap(errGo).With("file", outFn).With("value", matches[0][2]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if accu < acceptableVals[2] {
		return errors.New("accuracy is too small").With("file", outFn).With("line", scanner.Text()).With("value", accu).With("ceiling", acceptableVals[2]).With("stack", stack.Trace().TrimRuntime())
	}
	accu, errGo = strconv.ParseFloat(matches[0][4], 64)
	if errGo != nil {
		return errors.Wrap(errGo).With("file", outFn).With("value", matches[0][4]).With("line", scanner.Text()).With("stack", stack.Trace().TrimRuntime())
	}
	if accu < acceptableVals[3] {
		return errors.New("validation accuracy is too small").With("file", outFn).With("line", scanner.Text()).With("value", accu).With("ceiling", acceptableVals[3]).With("stack", stack.Trace().TrimRuntime())
	}

	logger.Info(matches[0][0])
	supressDump = true

	return nil
}

func downloadOutput(ctx context.Context, experiment *ExperData, output string) (err errors.Error) {

	archive, errGo := os.Create(output)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer archive.Close()

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	object, errGo := mc.GetObjectWithContext(ctx, experiment.Bucket, "output.tar", minio.GetObjectOptions{})
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if _, errGo = io.Copy(archive, object); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

type relocateTemp func() (err errors.Error)

type relocate struct {
	Original string
	Pop      []relocateTemp
}

func (r *relocate) Close() (err errors.Error) {
	if r == nil {
		return nil
	}
	// Iterate the list of call backs in reverse order when exiting
	// the stack of things that were done as a LIFO
	for i := len(r.Pop) - 1; i >= 0; i-- {
		if err = r.Pop[i](); err != nil {
			return err
		}
	}
	return nil
}

func relocateToTemp(dir string) (callback relocate, err errors.Error) {

	wd, errGo := os.Getwd()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	wd, errGo = filepath.Abs(dir)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if rel, _ := filepath.Rel(wd, dir); rel == "." {
		return errors.New("the relocation directory is the same directory as the target").With("dir", dir).With("current_dir", wd).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = os.Chdir(workDir); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	callback = cleanUp{
		Original: wd,
		Pop: []relocateTemp{func() (err errors.Error) {
			return os.Chdir(Original)
		}},
	}

	return callback, nil
}

func relocateToTransitory() (callback relocate, err errors.Error) {

	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if callback, err = relocateToTemp(dir); err != nil {
		return err
	}

	callback.Pop = append(callback.Pop, func() {
		// Move to an intermediate directory to allow the RemoveAll to occur
		os.Chdir(os.TempDir())
		return os.RemoveAll(dir)
	})

	return callback, nil
}

func TestRelocation(t *testing.T) {

	// Keep a record of the directory where we are currently located
	wd, errGo := os.Getwd()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	// Create a test directory
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
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
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if wd != newNW {
		t.Fatal(errors.New("relocation could not be reversed").With("origin", wd).With("recovered_to", newWD).With("temp_dir", dir).With("stack", stack.Trace().TrimRuntime()))
	}
}

func TestNewRelocation(t *testing.T) {

	// Keep a record of the directory where we are currently located
	wd, errGo := os.Getwd()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		if _, errGo = fl.WriteString(); errGo != nil {
			return errors.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
		}
		fl.Close()

		defer reloc.Close()
	}()

	// find out where we are and make sure it is where we expect
	newWD, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	// Make sure this was not a NOP
	if wd != newNW {
		t.Fatal(errors.New("relocation could not be reversed").With("origin", wd).With("recovered_to", newWD).With("temp_dir", dir).With("stack", stack.Trace().TrimRuntime()))
	}

	// Make sure our working directory was cleaned up
	if _, errGo := os.Stat(tmpDir); !os.IsNotExist(errGo) {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
}

// TestBasicRun is a function used to exercise the core ability of the runner to successfully
// complete a single experiment.  The name of the test uses a Latin A with Diaresis to order this
// test behind others that are simplier in nature.
//
// This test take a minute or two but is left to run in the short version of testing because
// it exercises the entire system under test end to end for experiments running in the python
// environment
//
func TestÄE2EExperimentRun(t *testing.T) {

	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	err := runner.IsAliveK8s()
	if err != nil {
		t.Fatal(err)
	}

	// Navigate to the assets directory being used for this experiment
	workDir, errGo := filepath.Abs(filepath.Join(wd, "..", "..", "assets", "tf_minimal"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	returnToWD, err := relocateToTemp(workDir)
	if err != nil {
		t.Fatal(err)
	}
	defer returnToWD.Clean()

	if err = setupRMQAdmin(t); err != nil {
		t.Fatal(err)
	}

	// Parse from the rabbitMQ Settings the username and password
	rmqURL, errGo := url.Parse(os.ExpandEnv(*amqpURL))
	if errGo != nil {
		t.Fatal(errGo)
	}

	// Place test files into the serving location for our minio server
	pass, _ := rmqURL.User.Password()
	experiment := &ExperData{
		RabbitMQUser:     rmqURL.User.Username(),
		RabbitMQPassword: pass,
		Bucket:           xid.New().String(),
		MinioAddress:     runner.MinioTest.Address,
		MinioUser:        runner.MinioTest.AccessKeyId,
		MinioPassword:    runner.MinioTest.SecretAccessKeyId,
	}

	// Read a template for the payload that will be sent to run the experiment
	payload, errGo := ioutil.ReadFile("experiment_template.json")
	if errGo != nil {
		t.Fatal(errGo)
	}
	tmpl, errGo := template.New("TestBasicRun").Parse(string(payload[:]))
	if errGo != nil {
		t.Fatal(errGo)
	}
	output := &bytes.Buffer{}
	if errGo = tmpl.Execute(output, experiment); errGo != nil {
		t.Fatal(errGo)
	}

	// Take the string template for the experiment and unmarshall it so that it can be
	// updated with live test data
	r, err := runner.UnmarshalRequest(output.Bytes())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Construct a json payload that uses the current wall clock time and also
	// refers to a locally embeded minio server
	r.Experiment.TimeAdded = float64(time.Now().Unix())
	r.Experiment.TimeLastCheckpoint = nil

	// Having constructed the payload identify the files within the tf_minimal directory and

	// save them into a workspace tar archive
	// Generate a tar file of the entire workspace directory and upload
	// to the minio server that the runner will pull from
	if err = uploadWorkspace(experiment); err != nil {
		t.Fatal(err)
	}

	defer runner.MinioTest.RemoveBucketAll(experiment.Bucket)

	// Now that the file needed is present on the minio server send the
	// experiment specification message to the worker using a new queue

	rmq, err := runner.NewRabbitMQ(*amqpURL, *amqpURL)
	if err != nil {
		t.Fatal(err)
	}

	queueType := "rmq"
	qName := queueType + "_" + xid.New().String()
	routingKey := "StudioML." + qName
	if err = rmq.QueueDeclare(qName); err != nil {
		t.Fatal(err)
	}

	b, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		t.Fatal(errGo)
	}

	// Send the payload to rabbitMQ
	err = rmq.Publish(routingKey, "application/json", b)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for prometheus to show the task as having been ran and completed
	pClient := NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", prometheusPort))

	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()

	// Run around checking the prometheus counters for our experiment seeing when the internal
	// project tracking says everything has completed, only then go out and get the experiment
	// results
	//
	func() {
		for {
			select {
			case <-tick.C:
				metrics, err := pClient.Fetch("runner_project_")
				if err != nil {
					t.Fatal(errors.Wrap(err).With("stack", stack.Trace().TrimRuntime()))
				}

				runningCnt, finishedCnt, err := projectStats(metrics, qName, queueType, r.Config.Database.ProjectId, r.Experiment.Key)
				if err != nil {
					t.Fatal(err)
				}

				// Wait for prometheus to show the task stopped for our specific queue, host, project and experiment ID
				if runningCnt == 0 && finishedCnt == 1 {
					return
				}
			}
		}
	}()

	// Query minio for the resulting output and compare it with the expected
	if err = validateExperiment(context.Background(), experiment); err != nil {
		t.Fatal(err)
	}

	// results that were bundled with the test file
}

// projectStats will take a collection of metrics, typically retrieved from a local prometheus
// source and scan these for details relating to a specific project and experiment
//
func projectStats(metrics map[string]*model.MetricFamily, qName string, qType string, project string, experiment string) (running int, finished int, err errors.Error) {
	for family, metric := range metrics {
		if metric.GetType() != model.MetricType_GAUGE {
			continue
		}
		if strings.HasPrefix(family, "runner_project_") {
			err = func() (err errors.Error) {
				vecs := metric.GetMetric()
				for _, vec := range vecs {
					for _, label := range vec.GetLabel() {
						switch label.GetName() {
						case "experiment":
							if label.GetValue() != experiment && len(experiment) != 0 {
								logger.Info("mismatched", "experiment", experiment, "value", label.GetValue())
								return nil
							}
						case "host":
							if label.GetValue() != host {
								logger.Info("mismatched", "host", host, "value", label.GetValue())
								return nil
							}
						case "project":
							if label.GetValue() != project {
								logger.Info("mismatched", "project", project, "value", label.GetValue())
								return nil
							}
						case "queue_type":
							if label.GetValue() != qType {
								logger.Info("mismatched", "qType", qType, "value", label.GetValue())
								return nil
							}
						case "queue_name":
							if !strings.HasSuffix(label.GetValue(), qName) {
								logger.Info("mismatched", "qName", qName, "value", label.GetValue())
								logger.Info(spew.Sdump(vecs))
								return nil
							}
						default:
							return nil
						}
					}

					// Based on the name of the gauge we will add together quantities, this
					// is done because the experiment might have been left out
					// of the inputs and the caller wanted a total for a project
					switch family {
					case "runner_project_running":
						running += int(vec.GetGauge().GetValue())
					case "runner_project_completed":
						finished += int(vec.GetGauge().GetValue())
					default:
					}
				}
				return nil
			}()
			if err != nil {
				return 0, 0, err
			}
		}
	}

	return running, finished, nil
}

func confirmOne(ctx context.Context, confirms <-chan amqp.Confirmation) (err errors.Error) {
	select {
	case <-ctx.Done():
		return errors.New("i/o timeout").With("stack", stack.Trace().TrimRuntime())
	case confirmed := <-confirms:
		if confirmed.Ack {
			return nil
		}
	}
	return nil
}
