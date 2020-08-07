// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of tests related to starting python based work and
// running it to completion within the server.  Work run is prepackaged within the source
// code repository and orchestrated by the testing within this file.

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
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

	"github.com/leaf-ai/studio-go-runner/internal/runner"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	"google.golang.org/protobuf/encoding/prototext"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	minio "github.com/minio/minio-go"

	"github.com/mholt/archiver"
	model "github.com/prometheus/client_model/go"
	"github.com/rs/xid"

	"github.com/makasim/amqpextra"
	rh "github.com/michaelklishin/rabbit-hole"
	"github.com/streadway/amqp"
)

var (
	reportQueuePrivatePEM = `-----BEGIN RSA PRIVATE KEY-----
Proc-Type: 4,ENCRYPTED
DEK-Info: AES-128-CBC,7F87EA7430471D8D3B7B3EAC1544BEF8

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
-----END RSA PRIVATE KEY-----
`
	reportQueuePublicPEM = `-----BEGIN RSA PUBLIC KEY-----
MIICCgKCAgEAvDNdV2+HzofKh0QUBp2gUhxhmxD/uXVZsEB6dk/yVhYepqHSMChg
YyQhriyxY6S7SinOd6QCm0Qe+bQEfX81e21PJ8BePjM66l4FgFaLEO7KKBLpZQdh
9dUQYbviuCiLr/4mj2GiShoMgPesLbcfLMy34mFLYRy93/EW5b8nzpMCbqh803Zc
RjBdc1HJu/fV5FW/awBAWCpduTYE0ozq80yRgr8bPKolWDGj5h/H6Np1lOjRZUdX
ksJ+dIlpKPjCyCbipSTyYZsrXMBprmxtLkPEMksaDgV2RbIviCBZTA3tg962LhPc
xLVzThEulrgrk6dCbtKYOhRDHzWyTl+akr7zFHz8FurFr8c2KWxUfgxIc17UbGG4
Vimh2JhrfdNDJVL7h06M+btsxlo8mdDzKy3sCjWjI6x1THjMthAtBl/RYbG8EgCm
AhUZ4L4cYVWLrd0Qd00DUOD/Wr7gEYq8UCN1FCwPT6296YiGnKr41wUAnAetB2x5
go4CsBQgp2VHN2+7OK4gLECAypfszk9voDtMbZawpy3gW6SkKyJ8JZ/jSMEFALc5
alm8E5l3GxTLZ7sp09Z/7nJGqHHyfB9sw5WKdH9uyx441SNMfgJXfwnImTuFnQmh
6/nogjltMjaWAbAbdMPyovffDtsUHcTxMayhrE+YO/omQNSY6xBq7xECAwEAAQ==
-----END RSA PUBLIC KEY-----
`

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
			t.Fatal(kv.NewError("a tensorflow result not extracted").With("expected", match).With("captured_match", matches[0][i]).With("stack", stack.Trace().TrimRuntime()))
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
	GPUs             []runner.GPUTrack
	GPUSlots         int
}

// downloadFile will download a url to a local file using streaming.
//
func downloadFile(fn string, download string) (err kv.Error) {

	// Create the file
	out, errGo := os.Create(fn)
	if errGo != nil {
		return kv.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer out.Close()

	// Get the data
	resp, errGo := http.Get(download)
	if errGo != nil {
		return kv.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer resp.Body.Close()

	// Write the body to file
	_, errGo = io.Copy(out, resp.Body)
	if errGo != nil {
		return kv.Wrap(errGo).With("url", download).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func downloadRMQCli(fn string) (err kv.Error) {
	if err = downloadFile(fn, os.ExpandEnv("http://${RABBITMQ_SERVICE_SERVICE_HOST}:${RABBITMQ_SERVICE_SERVICE_PORT_RMQ_ADMIN}/cli/rabbitmqadmin")); err != nil {
		return err
	}
	// Having downloaded the administration CLI tool set it to be executable
	if errGo := os.Chmod(fn, 0777); errGo != nil {
		return kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// setupRMQ will download the rabbitMQ administration tool from the k8s deployed rabbitMQ
// server and place it into the project bin directory setting it to executable in order
// that diagnostic commands can be run using the shell
//
func setupRMQAdmin() (err kv.Error) {
	rmqAdmin := path.Join("/project", "bin")
	fi, errGo := os.Stat(rmqAdmin)
	if errGo != nil {
		return kv.Wrap(errGo).With("dir", rmqAdmin).With("stack", stack.Trace().TrimRuntime())
	}
	if !fi.IsDir() {
		return kv.NewError("specified directory is not actually a directory").With("dir", rmqAdmin).With("stack", stack.Trace().TrimRuntime())
	}

	// Look for the rabbitMQ Server and download the command line tools for use
	// in diagnosing issues, and do this before changing into the test directory
	rmqAdmin = filepath.Join(rmqAdmin, "rabbitmqadmin")
	return downloadRMQCli(rmqAdmin)
}

func collectUploadFiles(dir string) (files []string, err kv.Error) {

	errGo := filepath.Walk(".",
		func(path string, info os.FileInfo, err error) error {
			files = append(files, path)
			return nil
		})

	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	sort.Strings(files)

	return files, nil
}

func uploadWorkspace(experiment *ExperData) (err kv.Error) {

	wd, _ := os.Getwd()
	logger.Debug("uploading", "dir", wd, "experiment", *experiment, "stack", stack.Trace().TrimRuntime())

	dir := "."
	files, err := collectUploadFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return kv.NewError("no files found").With("directory", dir).With("stack", stack.Trace().TrimRuntime())
	}

	// Pack the files needed into an archive within a temporary directory
	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(dir)

	archiveName := filepath.Join(dir, "workspace.tar")

	if errGo = archiver.Tar.Make(archiveName, files); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Now we have the workspace for upload go ahead and contact the minio server
	mc, errGo := minio.New(experiment.MinioAddress, experiment.MinioUser, experiment.MinioPassword, false)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	archive, errGo := os.Open(archiveName)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer archive.Close()

	fileStat, errGo := archive.Stat()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create the bucket that will be used by the experiment, and then place the workspace into it
	if errGo = mc.MakeBucket(experiment.Bucket, ""); errGo != nil {
		switch minio.ToErrorResponse(errGo).Code {
		case "BucketAlreadyExists":
		case "BucketAlreadyOwnedByYou":
		default:
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	_, errGo = mc.PutObject(experiment.Bucket, "workspace.tar", archive, fileStat.Size(),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func validateTFMinimal(ctx context.Context, experiment *ExperData) (err kv.Error) {
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

	object, errGo := mc.GetObjectWithContext(ctx, experiment.Bucket, "output.tar", minio.GetObjectOptions{})
	if errGo != nil {
		return kv.Wrap(errGo).With("output", output).With("stack", stack.Trace().TrimRuntime())
	}

	if _, errGo = io.Copy(archive, object); errGo != nil {
		return kv.Wrap(errGo).With("output", output).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

type relocateTemp func() (err kv.Error)

type relocate struct {
	Original string
	Pop      []relocateTemp
}

func (r *relocate) Close() (err kv.Error) {
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

func relocateToTemp(dir string) (callback relocate, err kv.Error) {

	wd, errGo := os.Getwd()
	if errGo != nil {
		return callback, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	dir, errGo = filepath.Abs(dir)
	if errGo != nil {
		return callback, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if rel, _ := filepath.Rel(wd, dir); rel == "." {
		return callback, kv.NewError("the relocation directory is the same directory as the target").With("dir", dir).With("current_dir", wd).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = os.Chdir(dir); errGo != nil {
		return callback, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	callback = relocate{
		Original: wd,
		Pop: []relocateTemp{func() (err kv.Error) {
			if errGo := os.Chdir(wd); errGo != nil {
				return kv.Wrap(errGo).With("dir", wd).With("stack", stack.Trace().TrimRuntime())
			}
			return nil
		}},
	}

	return callback, nil
}

func relocateToTransitory() (callback relocate, err kv.Error) {

	dir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return callback, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if callback, err = relocateToTemp(dir); err != nil {
		return callback, err
	}

	callback.Pop = append(callback.Pop, func() (err kv.Error) {
		// Move to an intermediate directory to allow the RemoveAll to occur
		if errGo := os.Chdir(os.TempDir()); errGo != nil {
			return kv.Wrap(errGo, "unable to retreat from the directory being deleted").With("dir", dir).With("stack", stack.Trace().TrimRuntime())
		}
		if errGo := os.RemoveAll(dir); errGo != nil {
			return kv.Wrap(errGo, "unable to retreat from the directory being deleted").With("dir", dir).With("stack", stack.Trace().TrimRuntime())
		}
		return nil
	})

	return callback, nil
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

// prepareExperiment reads an experiment template from the current working directory and
// then uses it to prepare the json payload that will be sent as a runner request
// data structure to a go runner
//
func prepareExperiment(gpus int, ignoreK8s bool) (experiment *ExperData, r *runner.Request, err kv.Error) {
	if !ignoreK8s {
		if err = setupRMQAdmin(); err != nil {
			return nil, nil, err
		}
	}

	// Parse from the rabbitMQ Settings the username and password that will be available to the templated
	// request
	rmqURL, errGo := url.Parse(os.ExpandEnv(*amqpURL))
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	slots := 0
	gpusToUse := []runner.GPUTrack{}
	if gpus != 0 {
		// Templates will also have access to details about the GPU cards, upto a max of three
		// so we find the gpu cards and if found load their capacity and allocation data into the
		// template data source.  These are used for live testing so use any live cards from the runner
		//
		invent, err := runner.GPUInventory()
		if err != nil {
			return nil, nil, err
		}
		if len(invent) < gpus {
			return nil, nil, kv.NewError("not enough gpu cards for a test").With("needed", gpus).With("actual", len(invent)).With("stack", stack.Trace().TrimRuntime())
		}

		// slots will be the total number of slots needed to grab the number of cards specified
		// by the caller
		if gpus > 1 {
			sort.Slice(invent, func(i, j int) bool { return invent[i].FreeSlots < invent[j].FreeSlots })

			// Get the largest n (gpus) cards that have free slots
			for i := 0; i != len(invent); i++ {
				if len(gpusToUse) >= gpus {
					break
				}
				if invent[i].FreeSlots <= 0 || invent[i].EccFailure != nil {
					continue
				}

				slots += int(invent[i].FreeSlots)
				gpusToUse = append(gpusToUse, invent[i])
			}
			if len(gpusToUse) < gpus {
				return nil, nil, kv.NewError("not enough available gpu cards for a test").With("needed", gpus).With("actual", len(gpusToUse)).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}
	// Find as many cards as defined by the caller and include the slots needed to claim them which means
	// we need the two largest cards to force multiple claims if needed.  If the  number desired is 1 or 0
	// then we dont do anything as the experiment template will control what we get

	// Place test files into the serving location for our minio server
	pass, _ := rmqURL.User.Password()
	experiment = &ExperData{
		RabbitMQUser:     rmqURL.User.Username(),
		RabbitMQPassword: pass,
		Bucket:           xid.New().String(),
		MinioAddress:     runner.MinioTest.Address,
		MinioUser:        runner.MinioTest.AccessKeyId,
		MinioPassword:    runner.MinioTest.SecretAccessKeyId,
		GPUs:             gpusToUse,
		GPUSlots:         slots,
	}

	// Read a template for the payload that will be sent to run the experiment
	payload, errGo := ioutil.ReadFile("experiment_template.json")
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	tmpl, errGo := template.New("TestBasicRun").Parse(string(payload[:]))
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	output := &bytes.Buffer{}
	if errGo = tmpl.Execute(output, experiment); errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Take the string template for the experiment and unmarshall it so that it can be
	// updated with live test data
	if r, err = runner.UnmarshalRequest(output.Bytes()); err != nil {
		return nil, nil, err
	}

	// If we are not using gpus then purge out the GPU sections of the request template
	if gpus == 0 {
		r.Experiment.Resource.Gpus = 0
		r.Experiment.Resource.GpuMem = ""
	}

	// Construct a json payload that uses the current wall clock time and also
	// refers to a locally embedded minio server
	r.Experiment.TimeAdded = float64(time.Now().Unix())
	r.Experiment.TimeLastCheckpoint = nil

	return experiment, r, nil
}

// projectStats will take a collection of metrics, typically retrieved from a local prometheus
// source and scan these for details relating to a specific project and experiment
//
func projectStats(metrics map[string]*model.MetricFamily, qName string, qType string, project string, experiment string) (running int, finished int, err kv.Error) {
	for family, metric := range metrics {
		switch metric.GetType() {
		case model.MetricType_GAUGE:
		case model.MetricType_COUNTER:
		default:
			continue
		}
		if strings.HasPrefix(family, "runner_project_") {
			err = func() (err kv.Error) {
				vecs := metric.GetMetric()
				for _, vec := range vecs {
					func() {
						for _, label := range vec.GetLabel() {
							switch label.GetName() {
							case "experiment":
								if label.GetValue() != experiment && len(experiment) != 0 {
									logger.Trace("mismatched", "experiment", experiment, "value", label.GetValue(), "stack", stack.Trace().TrimRuntime())
									return
								}
							case "host":
								if label.GetValue() != host {
									logger.Trace("mismatched", "host", host, "value", label.GetValue(), "stack", stack.Trace().TrimRuntime())
									return
								}
							case "project":
								if label.GetValue() != project {
									logger.Trace("mismatched", "project", project, "value", label.GetValue(), "stack", stack.Trace().TrimRuntime())
									return
								}
							case "queue_type":
								if label.GetValue() != qType {
									logger.Trace("mismatched", "qType", qType, "value", label.GetValue(), "stack", stack.Trace().TrimRuntime())
									return
								}
							case "queue_name":
								if !strings.HasSuffix(label.GetValue(), qName) {
									logger.Trace("mismatched", "qName", qName, "value", label.GetValue(), "stack", stack.Trace().TrimRuntime())
									logger.Trace(spew.Sdump(vecs))
									return
								}
							default:
								return
							}
						}

						logger.Trace("matched prometheus metric", "family", family, "vec", fmt.Sprint(*vec), "stack", stack.Trace().TrimRuntime())

						// Based on the name of the gauge we will add together quantities, this
						// is done because the experiment might have been left out
						// of the inputs and the caller wanted a total for a project
						switch family {
						case "runner_project_running":
							running += int(vec.GetGauge().GetValue())
						case "runner_project_completed":
							finished += int(vec.GetCounter().GetValue())
						default:
							logger.Info("unexpected", "family", family)
						}
					}()
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

type waitFunc func(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err kv.Error)

// waitForRun will check for an experiment to run using the prometheus metrics to
// track the progress of the experiment on a regular basis
//
func waitForRun(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err kv.Error) {
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
			if runningCnt == 0 && finishedCnt == 1 {
				return nil
			}
			interval = time.Duration(15 * time.Second)
		}
	}
}

func createResponseRMQ(qName string) (err kv.Error) {

	// Response queues always use encryption
	rmq, err := newRMQ(true)
	if err != nil {
		return err
	}

	if err = rmq.QueueDeclare(qName); err != nil {
		return err
	}

	return nil
}

func deleteResponseRMQ(qName string, queueType string, routingKey string) (err kv.Error) {
	rmq, err := newRMQ(false)
	if err != nil {
		return err
	}

	if err = rmq.QueueDestroy(qName); err != nil {
		return err
	}

	return nil
}

func newRMQ(encrypted bool) (rmq *runner.RabbitMQ, err kv.Error) {
	creds := ""

	qURL, errGo := url.Parse(os.ExpandEnv(*amqpURL))
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("url", *amqpURL).With("stack", stack.Trace().TrimRuntime())
	}
	if qURL.User != nil {
		creds = qURL.User.String()
	} else {
		return nil, kv.NewError("missing credentials in url").With("url", *amqpURL).With("stack", stack.Trace().TrimRuntime())
	}

	w, err := getWrapper()
	if encrypted {
		if err != nil {
			return nil, err
		}
	}

	qURL.User = nil
	return runner.NewRabbitMQ(qURL.String(), creds, w)
}

func marshallToRMQ(rmq *runner.RabbitMQ, qName string, r *runner.Request) (b []byte, err kv.Error) {
	if rmq == nil {
		return nil, kv.NewError("rmq uninitialized").With("stack", stack.Trace().TrimRuntime())
	}

	if !rmq.IsEncrypted() {
		buf, errGo := json.MarshalIndent(r, "", "  ")
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		return buf, nil
	}
	// To sign a message use a generated signing public key

	sigs := GetRqstSigs()
	sigDir := sigs.Dir()

	if len(sigDir) == 0 {
		return nil, kv.NewError("signatures directory not ready").With("stack", stack.Trace().TrimRuntime())
	}

	pubKey, prvKey, errGo := ed25519.GenerateKey(rand.Reader)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	sshKey, errGo := ssh.NewPublicKey(pubKey)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Write the public key
	keyFile := filepath.Join(sigDir, qName)
	if errGo = ioutil.WriteFile(keyFile, ssh.MarshalAuthorizedKey(sshKey), 0600); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Now wait for the signature package to signal that the keys
	// have been refreshed and our new file was there
	<-sigs.GetRefresh().Done()

	w, err := runner.KubernetesWrapper(*msgEncryptDirOpt)
	if err != nil {
		if runner.IsAliveK8s() != nil {
			return nil, err
		}
	}

	envelope, err := w.Envelope(r)
	if err != nil {
		return nil, err
	}

	envelope.Message.Fingerprint = ssh.FingerprintSHA256(sshKey)

	sig, errGo := prvKey.Sign(rand.Reader, []byte(envelope.Message.Payload), crypto.Hash(0))
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Encode the base signature into two fields with binary length fromatted
	// using the SSH RFC method
	envelope.Message.Signature = base64.StdEncoding.EncodeToString(sig)

	if b, errGo = json.MarshalIndent(envelope, "", "  "); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return b, nil
}

// publishToRMQ will marshall a go structure containing experiment parameters and
// environment information and then send it to the rabbitMQ server this server is configured
// to listen to
//
func publishToRMQ(qName string, queueType string, routingKey string, r *runner.Request, encrypted bool) (err kv.Error) {
	rmq, err := newRMQ(encrypted)
	if err != nil {
		return err
	}

	if err = rmq.QueueDeclare(qName); err != nil {
		return err
	}

	b, err := marshallToRMQ(rmq, qName, r)

	// Send the payload to rabbitMQ
	return rmq.Publish(routingKey, "application/json", b)
}

func watchResponseQueue(ctx context.Context, qName string, prvKey *rsa.PrivateKey) (msgQ chan *runnerReports.Report, err kv.Error) {
	deliveryC := make(chan *runnerReports.Report)

	// Response queues are always encrypted
	rmq, err := newRMQ(true)
	if err != nil {
		return nil, err
	}

	mgmt, err := rmq.AttachMgmt(10 * time.Second)
	if err != nil {
		logger.Info("queue management unavailable", "error", err)
	}
	if mgmt != nil {
		go func(ctx context.Context, mgmt *rh.Client, qName string) {
			for {
				select {
				case <-time.After(2 * time.Second):
					q, errGo := mgmt.GetQueue("/", qName)
					if errGo != nil {
						logger.Info("mgmt get queue failed", "queue_name", qName, "error", errGo.Error())
						continue
					}
					logger.Info("queue stats", "queue", spew.Sdump(q))
				case <-ctx.Done():
					return
				}
			}
		}(ctx, mgmt, qName)
	}

	conn := amqpextra.Dial([]string{rmq.URL()})
	conn.SetLogger(amqpextra.LoggerFunc(log.Printf))

	consumer := conn.Consumer(
		qName,
		amqpextra.WorkerFunc(func(ctx context.Context, msg amqp.Delivery) interface{} {
			// process message
			payload, err := runner.Unseal(msg.ContentEncoding, prvKey)
			if err != nil {
				logger.Warn("invalid report received", "error", err)
				return err
			}

			report := &runnerReports.Report{}
			if err := prototext.Unmarshal(payload, report); err != nil {
				logger.Warn("invalid report received", "error", err)
				return err
			}

			if report == nil {
				logger.Info("nil report received")
				return nil
			}

			logger.Info("report received", "report", spew.Sdump(*report))

			select {
			case deliveryC <- report:
			case <-time.After(5 * time.Second):
				msg.Ack(false)
				return nil
			}

			msg.Ack(true)

			return nil
		}),
	)
	consumer.SetWorkerNum(1)
	consumer.SetContext(ctx)

	go consumer.Run()
	return deliveryC, nil
}

func pullReports(ctx context.Context, qName string, msgC <-chan *runnerReports.Report) {
	for {
		select {
		case msg := <-msgC:
			if msg == nil {
				fmt.Println("Decryption nothing left to watch", stack.Trace().TrimRuntime())
				return
			}
			// Implement decryption
			fmt.Println("Decryption required", spew.Sdump(*msg), stack.Trace().TrimRuntime())
		case <-ctx.Done():
			return
		}
	}
}

type validationFunc func(ctx context.Context, experiment *ExperData) (err kv.Error)

// runStudioTest will run a python based experiment and will then present the result to
// a caller supplied validation function
//
func runStudioTest(ctx context.Context, workDir string, gpus int, ignoreK8s bool, useEncryption bool, waiter waitFunc, validation validationFunc) (err kv.Error) {

	if !ignoreK8s {
		if err = runner.IsAliveK8s(); err != nil {
			return err
		}
	}

	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	// Check that the minio local server has initialized before continuing
	if alive, err := runner.MinioTest.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			return err
		}
		return kv.NewError("The minio test server is not available to run this test").With("stack", stack.Trace().TrimRuntime())
	}
	logger.Debug("alive checked", "addr", runner.MinioTest.Address)

	// Handle path for the response encryption before relocation to a temp
	// directory occurs
	keyPath, errGo := filepath.Abs(*sigsRspnsDirOpt)
	if errGo != nil {
		return kv.Wrap(errGo).With("dir", *sigsRspnsDirOpt).With("stack", stack.Trace().TrimRuntime())
	}

	returnToWD, err := relocateToTemp(workDir)
	if err != nil {
		return err
	}
	defer returnToWD.Close()

	logger.Debug("test relocated", "workDir", workDir)

	experiment, r, err := prepareExperiment(gpus, ignoreK8s)
	if err != nil {
		return err
	}

	// Having constructed the payload identify the files within the test template
	// directory and save them into a workspace tar archive then
	// generate a tar file of the entire workspace directory and upload
	// to the minio server that the runner will pull from
	if err = uploadWorkspace(experiment); err != nil {
		return err
	}

	logger.Debug("experiment uploaded")

	// Cleanup the bucket only after the validation function that was supplied has finished
	defer runner.MinioTest.RemoveBucketAll(experiment.Bucket)

	// Generate queue names that will be used for this test case
	queueType := "rmq"
	qName := queueType + "_Multipart_" + xid.New().String()
	routingKey := "StudioML." + qName

	// Use the preloaded private pair for use with
	// response queues.

	// First load the public key for local testing use that will encrypt the response message
	// Set a secret both using Kubernetes and also the locally populated store
	if err := runner.K8sUpdateSecret("studioml-report-keys", qName, []byte(reportQueuePublicPEM)); err != nil {
		return err
	}

	if errGo := os.MkdirAll(keyPath, 0700); errGo != nil {
		return kv.Wrap(errGo).With("dir", keyPath).With("stack", stack.Trace().TrimRuntime())
	}
	fn := filepath.Join(keyPath, qName+"_response")
	if errGo := ioutil.WriteFile(fn, []byte(reportQueuePublicPEM), 0600); errGo != nil {
		return kv.Wrap(errGo).With("fn", fn).With("stack", stack.Trace().TrimRuntime())
	}

	// Get and wait for the outgoing encryption loader to locate our new key
	if GetRspnsEncrypt() == nil {
		return kv.NewError("uninitialized").With("stack", stack.Trace().TrimRuntime())
	}
	for {
		if len(GetRspnsEncrypt().Dir()) != 0 {
			if GetRspnsEncrypt().GetRefresh() != nil {
				break
			}
		}
		time.Sleep(5 * time.Second)
	}
	<-GetRspnsEncrypt().GetRefresh().Done()

	// Retrieve the private key and use it inside the testing
	prvPEM, _ := pem.Decode([]byte(reportQueuePrivatePEM))
	pemBytes, errGo := x509.DecryptPEMBlock(prvPEM, []byte("PassPhrase"))
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	prvKey, errGo := x509.ParsePKCS1PrivateKey(pemBytes)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create and listen to the response queue which will receive messages
	// from the worker
	respQName := qName + "_response"
	if err = createResponseRMQ(respQName); err != nil {
		return err
	}
	defer deleteResponseRMQ(respQName, queueType, routingKey)
	logger.Debug("created response queue", "queue", respQName)

	responseCtx, cancelResponse := context.WithCancel(context.Background())
	defer cancelResponse()

	msgC, err := watchResponseQueue(responseCtx, respQName, prvKey)
	if err != nil {
		return err
	}

	go pullReports(responseCtx, respQName, msgC)

	logger.Debug("test initiated", "queue", qName, "stack", stack.Trace().TrimRuntime())

	// Now that the file needed is present on the minio server send the
	// experiment specification message to the worker using a new queue

	if err = publishToRMQ(qName, queueType, routingKey, r, useEncryption); err != nil {
		return err
	}

	logger.Debug("test waiting", "queue", qName, "stack", stack.Trace().TrimRuntime())

	if err = waiter(ctx, qName, queueType, r, prometheusPort); err != nil {
		return err
	}

	// Query minio for the resulting output and compare it with the expected
	return validation(ctx, experiment)
}

// TestÄE2EExperimentRun is a function used to exercise the core ability of the runner to successfully
// complete a single experiment.  The name of the test uses a Latin A with Diaresis to order this
// test after others that are simpler in nature.
//
// This test take a minute or two but is left to run in the short version of testing because
// it exercises the entire system under test end to end for experiments running in the python
// environment
//
func TestÄE2ECPUExperimentRun(t *testing.T) {
	E2EExperimentRun(t, 0)
}

func TestÄE2EGPUExperimentRun(t *testing.T) {
	if !*runner.UseGPU {
		logger.Warn("TestÄE2EExperimentRun not run")
		t.Skip("GPUs disabled for testing")
	}
	E2EExperimentRun(t, 1)

}

func E2EExperimentRun(t *testing.T, gpusNeeded int) {

	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	gpuCount := runner.GPUCount()
	if gpusNeeded > gpuCount {
		t.Skipf("insufficient GPUs %d, needed %d", gpuCount, gpusNeeded)
	}

	cases := []struct {
		useEncrypt bool
	}{
		{useEncrypt: true},
		{useEncrypt: false},
	}

	for _, aCase := range cases {
		wd, errGo := os.Getwd()
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
		}
		// Navigate to the assets directory being used for this experiment
		workDir, errGo := filepath.Abs(filepath.Join(wd, "..", "..", "assets", "tf_minimal"))
		if errGo != nil {
			t.Fatal(errGo)
		}

		if err := runStudioTest(context.Background(), workDir, gpusNeeded, false, aCase.useEncrypt, waitForRun, validateTFMinimal); err != nil {
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

func validatePytorchMultiGPU(ctx context.Context, experiment *ExperData) (err kv.Error) {
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

	if !*useK8s {
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

	if err := runStudioTest(context.Background(), workDir, 2, false, false, waitForRun, validatePytorchMultiGPU); err != nil {
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
