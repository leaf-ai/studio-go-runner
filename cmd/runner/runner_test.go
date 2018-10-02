package main

import (
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
	"sort"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	minio "github.com/minio/minio-go"
	"github.com/streadway/amqp"

	"github.com/mholt/archiver"
	"github.com/rs/xid"
)

// This file contains the implementation of tests related to starting python based work and
// running it to completion within the server.  Work run is prepackaged within the source
// code repository and orchestrated by the testing within this file.

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

// TestBasicRun is a function used to exercise the core ability of the runner to successfully
// complete a single experiment
//
func TestBasicRun(t *testing.T) {

	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	err := runner.IsAliveK8s()
	if err != nil {
		t.Fatal(err)
	}

	if err = setupRMQAdmin(t); err != nil {
		t.Fatal(err)
	}

	// Navigate to the assets directory being used for this experiment
	wd, errGo := os.Getwd()
	if errGo != nil {
		t.Fatal(errGo)
	}
	defer os.Chdir(wd)
	workDir, errGo := filepath.Abs(filepath.Join(wd, "..", "..", "assets", "tf_minimal"))
	if errGo != nil {
		t.Fatal(errGo)
	}

	if errGo = os.Chdir(workDir); errGo != nil {
		t.Fatal(errGo)
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

	// Having constructed the payload identify the files within the tf_minimal directory
	files := []string{}
	errGo = filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			files = append(files, path)
			return nil
		})
	if errGo != nil {
		t.Fatal(errGo)
	}
	sort.Strings(files)

	// Pack the files needed into an archive within a temporary directory
	dir, errGo := ioutil.TempDir("", "TestBasicRun")
	if errGo != nil {
		t.Fatal(errGo)
	}
	defer os.RemoveAll(dir)
	archiveName := filepath.Join(dir, "workspace.tar")

	if errGo = archiver.Tar.Make(archiveName, files); errGo != nil {
		t.Fatal(errGo)
	}

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		t.Fatal(errGo)
	}

	archive, errGo := os.Open(archiveName)
	if errGo != nil {
		t.Fatal(errGo)
	}
	defer archive.Close()

	fileStat, errGo := archive.Stat()
	if errGo != nil {
		t.Fatal(errGo)
	}

	// Create the bucket that will be used by the experiment, and then place the workspace into it
	if errGo = mc.MakeBucket(experiment.Bucket, ""); errGo != nil {
		switch minio.ToErrorResponse(errGo).Code {
		case "BucketAlreadyExists":
		case "BucketAlreadyOwnedByYou":
		default:
			t.Fatal(errGo)
		}
	}

	_, errGo = mc.PutObject(experiment.Bucket, "workspace.tar", archive, fileStat.Size(),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		t.Fatal(errGo)
	}
	defer runner.MinioTest.RemoveBucketAll(experiment.Bucket)

	// Now that the file needed is present on the minio server send the
	// experiment specification message to the worker using a new queue

	rmq, err := runner.NewRabbitMQ(*amqpURL, *amqpURL)
	if err != nil {
		t.Fatal(err)
	}
	qName := "rmq_" + xid.New().String()
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

	// Wait for prometheus to show the task running
	pClient := NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", prometheusPort))

	timeout := time.After(5 * time.Minute)
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-timeout:
			break
		case <-tick:
			metrics, err := pClient.Fetch("runner_")
			if err != nil {
				return errors.Wrap(err).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}

	// Wait for prometheus to show the task stopped
	// time.Sleep(5 * time.Minute)
	// defer logger.Warn(string(b))
	// Watch minio for the resulting output and compare it with the expected
	// results that were bundled with the test files
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
