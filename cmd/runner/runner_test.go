package main

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	minio "github.com/minio/minio-go"

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

// TestBasicRun is a function used to exercise the core ability of the runner to successfully
// complete a single experiment
//
func TestBasicRun(t *testing.T) {

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

	_, errGo = mc.PutObject(experiment.Bucket, "workspace.tar", archive, fileStat.Size(),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		t.Fatal(errGo)
	}

	// Now that the file needed is present on the minio server send the
	// experiment specification message to the worker using a new queue

	rmq, err := runner.NewRabbitMQ(*amqpURL, *amqpURL)
	if err != nil {
		t.Fatal(err)
	}
	qName := "rmq_" + xid.New().String()
	//routingKey := "StudioML." + qName
	if err = rmq.QueueDeclare(qName); err != nil {
		t.Fatal(err)
	}

	b, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		t.Fatal(errGo)
	}
	logger.Warn(string(b))

	// Send the payload to rabbitMQ

	// Watch minio for the resulting output and compare it with the expected
	// results that were bundled with the test files
}
