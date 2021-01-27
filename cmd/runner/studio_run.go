// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
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
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	htmlTemplate "html/template"
	textTemplate "text/template"

	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"

	minio_local "github.com/leaf-ai/go-service/pkg/minio"
	"github.com/leaf-ai/go-service/pkg/server"

	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	model "github.com/prometheus/client_model/go"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/jjeffery/kv"
	"github.com/makasim/amqpextra"
	"github.com/mholt/archiver"
	rh "github.com/michaelklishin/rabbit-hole"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/otiai10/copy"
	"github.com/rs/xid"
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
)

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
		callback = relocate{
			Pop: []relocateTemp{func() (err kv.Error) {
				return nil
			}},
		}
		return callback, nil
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

// prepareExperiment reads an experiment template from the current working directory and
// then uses it to prepare the json payload that will be sent as a runner request
// data structure to a go runner
//
func prepareExperiment(gpus int, mts *minio_local.MinioTestServer, ignoreK8s bool) (experiment *ExperData, r *runner.Request, err kv.Error) {
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
		MinioAddress:     mts.Address,
		MinioUser:        mts.AccessKeyId,
		MinioPassword:    mts.SecretAccessKeyId,
		GPUs:             gpusToUse,
		GPUSlots:         slots,
	}

	// Read a template for the payload that will be sent to run the experiment
	payload, errGo := ioutil.ReadFile("experiment_template.json")
	if errGo != nil {
		return nil, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	tmpl, errGo := htmlTemplate.New("TestBasicRun").Parse(string(payload[:]))
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

	// Replace the fixed experiment key with a new random to prevent test experiments
	// colliding in parallel test runs
	//
	r.Experiment.Key = xid.New().String()

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
	mc, errGo := minio.New(experiment.MinioAddress, &minio.Options{
		Creds:  credentials.NewStaticV4(experiment.MinioUser, experiment.MinioPassword, ""),
		Secure: false,
	})
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
	if errGo = mc.MakeBucket(context.Background(), experiment.Bucket, minio.MakeBucketOptions{}); errGo != nil {
		switch minio.ToErrorResponse(errGo).Code {
		case "BucketAlreadyExists":
		case "BucketAlreadyOwnedByYou":
		default:
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	_, errGo = mc.PutObject(ctx, experiment.Bucket, "workspace.tar", archive, fileStat.Size(),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

type waitFunc func(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err kv.Error)

// waitForRun will check for an experiment to run using the prometheus metrics to
// track the progress of the experiment on a regular basis
//
func waitForRun(ctx context.Context, qName string, queueType string, r *runner.Request, prometheusPort int) (err kv.Error) {
	// Wait for prometheus to show the task as having been ran and completed
	pClient := runner.NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", prometheusPort))

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

	logger.Debug("created queue", qName, "stack", stack.Trace().TrimRuntime())
	return nil
}

func deleteResponseRMQ(qName string, queueType string) (err kv.Error) {
	rmq, err := newRMQ(false)
	if err != nil {
		return err
	}

	if err = rmq.QueueDestroy(qName); err != nil {
		return err
	}

	logger.Debug("deleted queue", qName, "stack", stack.Trace().TrimRuntime())
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
		if server.IsAliveK8s() != nil {
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
func publishToRMQ(qName string, r *runner.Request, encrypted bool) (err kv.Error) {
	rmq, err := newRMQ(encrypted)
	if err != nil {
		return err
	}

	if err = rmq.QueueDeclare(qName); err != nil {
		return err
	}

	b, err := marshallToRMQ(rmq, qName, r)
	if err != nil {
		return err
	}

	// Send the payload to rabbitMQ
	return rmq.Publish("StudioML."+qName, "application/json", b)
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
			pubCnt := int64(0)
			dlvrCnt := int64(0)
			dlvrNoAckCnt := int64(0)
			for {
				select {
				case <-time.After(30 * time.Second):
					q, errGo := mgmt.GetQueue("/", qName)
					if errGo != nil {
						logger.Info("mgmt get queue failed", "queue_name", qName, "error", errGo.Error())
						continue
					}
					if q.MessageStats.Publish != pubCnt || q.MessageStats.Deliver != dlvrCnt || q.MessageStats.DeliverNoAck != dlvrNoAckCnt {
						logger.Info("queue stats", "queue_name", qName, "published", q.MessageStats.Publish,
							"working", dlvrNoAckCnt, "done", dlvrCnt)
						pubCnt = q.MessageStats.Publish
						dlvrCnt = q.MessageStats.Deliver
						dlvrNoAckCnt = q.MessageStats.DeliverNoAck
					}

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
			if len(msg.Body) == 0 {
				debugMsg := spew.Sdump(msg)
				if len(debugMsg) > 1024 {
					debugMsg = debugMsg[:1023]
				}
				logger.Warn("empty report received", spew.Sdump(msg))
				return nil
			}

			// process message
			payload, err := runner.Unseal(string(msg.Body), prvKey)
			if err != nil {
				if len(msg.Body) > 64 {
					logger.Warn("invalid report received", spew.Sdump(msg.Body[:64]))
				} else {
					logger.Warn("invalid report received", spew.Sdump(msg.Body))
				}
				return err
			}

			report := &runnerReports.Report{}
			if err := protojson.Unmarshal(payload, report); err != nil {
				logger.Warn("invalid report received", "error", err)
				return err
			}

			if report == nil {
				logger.Info("nil report received")
				return nil
			}

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

func pullReports(ctx context.Context, qName string, msgC <-chan *runnerReports.Report) (rpts []*reports.Report) {
	rpts = []*reports.Report{}

	for {
		select {
		case msg := <-msgC:
			if msg == nil {
				logger.Info("nothing left to watch", stack.Trace().TrimRuntime())
				return
			}
			generatedAt, errGo := ptypes.Timestamp(msg.Time)
			if errGo != nil {
				// If we can report the error to the test watcher
				logger.Warn("bad timestamp report sent", "error", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
				continue
			}
			if logger.IsTrace() {
				logger.Trace("report received", "experiment id", msg.GetExperimentId(), "generated at", generatedAt.String(),
					"host", msg.GetExecutorId(), "report", msg.String(), "stack", stack.Trace().TrimRuntime())
			}
			rpts = append(rpts, msg)
		case <-ctx.Done():
			return
		}
	}
	return rpts
}

type validationFunc func(ctx context.Context, experiment *ExperData, rpts []*reports.Report, pythonSidecarLogs []string) (err kv.Error)

type studioRunOptions struct {
	WorkDir       string // The directory where the experiment is homed
	AssetDir      string // The directory in which the assets used for testing can be found
	QueueName     string
	mts           *minio_local.MinioTestServer
	GPUs          int
	NoK8sCheck    bool
	UseEncryption bool
	SendReports   bool           // Report messages are to be sent using a response queue
	ListenReports bool           // Use a Go implementation of a listener for report messages
	PythonReports bool           // Use a python implementation of a listener for report messages
	Waiter        waitFunc       // Custom wait function for experiment progress monitoring
	Validation    validationFunc // Validation function for asserting the results of the test
}

// studioRun will run a python based experiment and will then present the result to
// a caller supplied validation function
//
func studioRun(ctx context.Context, opts studioRunOptions) (err kv.Error) {

	if !opts.NoK8sCheck {
		if err = server.IsAliveK8s(); err != nil {
			return err
		}
	}

	if len(opts.WorkDir) == 0 {
		return kv.NewError("The test WorkDir was not specified").With("stack", stack.Trace().TrimRuntime())
	}

	if len(opts.AssetDir) == 0 {
		return kv.NewError("The test AssetDir was not specified").With("stack", stack.Trace().TrimRuntime())
	}

	if opts.ListenReports && !opts.SendReports {
		return kv.NewError("internal report listener enabled without send reports enabled").With("stack", stack.Trace().TrimRuntime())
	}

	if opts.PythonReports && !opts.SendReports {
		return kv.NewError("python reports listener enabled without send reports enabled").With("stack", stack.Trace().TrimRuntime())
	}

	if opts.PythonReports && opts.ListenReports {
		return kv.NewError("both the Go and python reports listener enabled unexpectedly").With("stack", stack.Trace().TrimRuntime())
	}

	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	// Check that the minio local server has initialized before continuing
	if alive, err := opts.mts.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			return err
		}
		return kv.NewError("The minio test server is not available to run this test").With("stack", stack.Trace().TrimRuntime())
	}
	logger.Debug("alive checked", "addr", opts.mts.Address)

	// Handle path for the response encryption before relocation to a temp
	// directory occurs
	keyPath, errGo := filepath.Abs(*sigsRspnsDirOpt)
	if errGo != nil {
		return kv.Wrap(errGo).With("dir", *sigsRspnsDirOpt).With("stack", stack.Trace().TrimRuntime())
	}

	// Changes the working dir to be our working dir, no file copying in this
	returnToWD, err := relocateToTemp(opts.WorkDir)
	if err != nil {
		return err
	}
	defer returnToWD.Close()

	logger.Debug("test relocated", "workDir", opts.WorkDir)

	// prepareExperiment sets up the queue and loads the experiment
	// metadata request
	experiment, r, err := prepareExperiment(opts.GPUs, opts.mts, opts.NoK8sCheck)
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
	defer opts.mts.RemoveBucketAll(experiment.Bucket)

	// Generate queue names that will be used for this test case
	qType := "rmq"
	qName := qType + "_StudioRun_" + xid.New().String()
	if len(opts.QueueName) != 0 {
		parts := strings.Split(opts.QueueName, "_")
		qType = parts[0]
		qName = opts.QueueName
	}

	// The response queue private key needs to be carried between two if statements
	// controlling the respon queue feature so declare it for the entire function
	prvKey := &rsa.PrivateKey{}

	// Use the preloaded key pair for use with response queue encryption.
	if opts.SendReports {
		if prvKey, err = prepReportingKey(opts, keyPath, qName); err != nil {
			return err
		}
	}

	return studioExecute(ctx, opts, experiment, qName, qType, prvKey, r)
}

func prepReportingKey(opts studioRunOptions, keyPath string, qName string) (prvKey *rsa.PrivateKey, err kv.Error) {
	// First load the public key for local testing use that will encrypt the response message
	// Set a secret both using Kubernetes and also the locally populated store
	if err := server.IsAliveK8s(); err != nil && !opts.NoK8sCheck {
		if err := server.K8sUpdateSecret("studioml-report-keys", qName, []byte(reportQueuePublicPEM)); err != nil {
			return nil, err
		}
	}

	if errGo := os.MkdirAll(keyPath, 0700); errGo != nil {
		return nil, kv.Wrap(errGo).With("dir", keyPath).With("stack", stack.Trace().TrimRuntime())
	}
	fn := filepath.Join(keyPath, qName+"_response")
	if errGo := ioutil.WriteFile(fn, []byte(reportQueuePublicPEM), 0600); errGo != nil {
		return nil, kv.Wrap(errGo).With("fn", fn).With("stack", stack.Trace().TrimRuntime())
	}

	// Get and wait for the outgoing encryption loader to locate our new key
	if GetRspnsEncrypt() == nil {
		return nil, kv.NewError("uninitialized").With("stack", stack.Trace().TrimRuntime())
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
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if prvKey, errGo = x509.ParsePKCS1PrivateKey(pemBytes); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return prvKey, nil
}

func studioExecute(ctx context.Context, opts studioRunOptions, experiment *ExperData,
	qName string, qType string, prvKey *rsa.PrivateKey, r *runner.Request) (err kv.Error) {

	// rptsC is used to send any reports that have been received during testing
	rptsC := make(chan []*reports.Report, 1)
	defer close(rptsC)

	pythonListenerC := make(chan []string, 1)
	defer close(pythonListenerC)

	stopReports, cancelReports := context.WithCancel(context.Background())

	respQName := qName + "_response"
	reportDone, err := doReports(stopReports, respQName, qType, opts, prvKey, rptsC, pythonListenerC)
	if err != nil {
		return err
	}

	logger.Debug("test initiated", "queue", qName, "stack", stack.Trace().TrimRuntime())

	rpts := []*reports.Report{}
	pythonLogs := []string{}

	// A function is used to allow for the defer of the cancelReports
	// context
	err = func() (err kv.Error) {
		// Generate an ID the running experiment can use to identify repeated runs during
		// testing
		r.Config.Env["RUN_ID"] = xid.New().String()

		// Now that the file needed is present on the minio server send the
		// experiment specification message to the worker using a new queue

		if err = publishToRMQ(qName, r, opts.UseEncryption); err != nil {
			return err
		}

		logger.Debug("test waiting", "queue", qName, "stack", stack.Trace().TrimRuntime())

		if err = opts.Waiter(ctx, qName, qType, r, server.GetPrometheusPort()); err != nil {
			return err
		}

		// The response queue needs deleting here so that the python listener stops
		cancelReports()

		// Now wait for the reporting to finish after the experiment is done
		if reportDone != nil {
			logger.Debug("test over waiting on reports", "queue", qName, "stack", stack.Trace().TrimRuntime())
			select {
			case <-reportDone:
				logger.Debug("test over reports ready", "queue", qName, "stack", stack.Trace().TrimRuntime())
			case <-time.After(time.Minute):
			}
		}

		// Now the waiter is done go and retrieve any reports, after a second of idle time
		// assume all reports have been retrieved and continue
		if opts.ListenReports {
			logger.Debug("retrieve reports", "queue", qName, "stack", stack.Trace().TrimRuntime())
			rpts = func(rptsC chan []*reports.Report) (rpts []*reports.Report) {
				rpts = []*reports.Report{}
				for {
					select {
					case r := <-rptsC:
						// If there was no data or the channel is closed continue
						if r == nil {
							logger.Debug("report fetch abandoned due to nil")
							return rpts
						}
						rpts = append(rpts, r...)
					case <-time.After(5 * time.Second):
						logger.Debug("report fetch timed out")
						return rpts
					}
				}
			}(rptsC)
		} else {
			logger.Debug("retrieve python listener logs", "queue", qName, "stack", stack.Trace().TrimRuntime())
			pythonLogs = func(logsC chan []string) (pythonLogs []string) {
				pythonLogs = []string{}
				for {
					select {
					case log := <-logsC:
						// If there was no data or the channel is closed continue
						if r == nil {
							logger.Debug("python logs fetch abandoned due to nil")
							return pythonLogs
						}
						pythonLogs = append(pythonLogs, log...)
					case <-time.After(3 * time.Second):
						logger.Debug("python logs fetch timed out")
						return pythonLogs
					}
				}
			}(pythonListenerC)
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Query minio for the resulting output and compare it with the expected
	return opts.Validation(ctx, experiment, rpts, pythonLogs)
}

// doReports will perform the actions needed to capture the report channel traffic related to
// realtime experiment tracking.
//
// done is used when there are asynchronous actions that need to complete after the experiment
// is done.  The client can use this to know when reports have been pulled and are available.  If
// done is not returned then this implies asynhcornous processing is not needed to complete
// report pulling and the client can proceed immmately after the experiment is done tio handle
// the data structure chosen to deal with reports being returned.
//
func doReports(ctx context.Context, qName string, qType string, opts studioRunOptions, prvKey *rsa.PrivateKey, rptsC chan []*reports.Report, logsC chan []string) (done chan struct{}, err kv.Error) {

	if opts.SendReports {
		// Create and listen to the response queue which will receive messages
		// from the worker
		if err = createResponseRMQ(qName); err != nil {
			return nil, err
		}

		// We dont want the response queue to just be dropped once this function returns.
		// Instead a wait group is used to indicate that the processing done by the experiment has
		// completed and the results scrapped before the response queue is to be deleted.

		dropResponse := sync.WaitGroup{}

		defer func() {
			// The next part is blocking so to prevent the doReports from blocking
			// we put the cleanup into a go func
			go func() {
				// Wait for the experiment termination to delete the response queue
				<-ctx.Done()

				logger.Debug("doReports release response queue", "stack", stack.Trace().TrimRuntime())
				deleteResponseRMQ(qName, qType)
			}()
		}()

		switch {
		case opts.ListenReports:
			logger.Debug("created listener response queue", "queue", qName)

			msgC, err := watchResponseQueue(ctx, qName, prvKey)
			if err != nil {
				return nil, err
			}

			// Create a channel that the report feature can use to pass back validated reports
			// for when the validation occurs
			dropResponse.Add(1)
			go func() {
				defer func() {
					_ = recover()
					logger.Debug("doReports release", "stack", stack.Trace().TrimRuntime())
					dropResponse.Done()
				}()
				rptsC <- pullReports(ctx, qName, msgC)
				logger.Debug("pull reports done", "queue_name", qName)
			}()
		case opts.PythonReports:
			// PythonReports uses a sample python implementation of
			// a queue listener for report messages

			logger.Debug("created python response queue", "queue", qName)
			src, errGo := filepath.Abs(filepath.Join(opts.AssetDir, "response_catcher"))
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			// Create a new TMPDIR because the python pip tends to leave dirt behind
			// when doing pip builds etc
			tmpDir, errGo := ioutil.TempDir("", "response-queue")
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			defer func() {
				// Use the defer to queue up a cleanup function that will wait
				// asynchronously for the main python response process and other related
				// processing in other parts of the function to complete and then
				// clean up
				go func() {
					dropResponse.Wait()
					if !*debugOpt {
						logger.Warn("deleting python response queue temp dir", "tmp_dir", tmpDir)
						os.RemoveAll(tmpDir)
					} else {
						logger.Info("python response queue temp dir retained", "tmp_dir", tmpDir)
					}
				}()
			}()

			// Copy the standard minimal tensorflow test into a working directory
			if errGo = copy.Copy(src, tmpDir); errGo != nil {
				return nil, kv.Wrap(errGo).With("src", src, "tmp", tmpDir).With("stack", stack.Trace().TrimRuntime())
			}

			rmqHost := "localhost"
			rmqUser := "guest"
			rmqPassword := "guest"

			if server.IsAliveK8s() == nil {
				rmqHost = "${RABBITMQ_SERVICE_SERVICE_HOST}"
				rmqUser = "${RABBITMQ_DEFAULT_USER}"
				rmqPassword = "${RABBITMQ_DEFAULT_PASS}"
			}
			outputFN := filepath.Join(tmpDir, "responses")
			// Generate a script file with command line options filled in as appropriate
			// and place the file directly into the tmpDir
			respCmd := `#!/bin/bash
PYENV_VERSION=3.6
IFS=$'\n'; arr=( $(pyenv versions --bare | grep -v studioml || true) )
for i in ${arr[@]} ; do
 if [[ "$i" == ${PYENV_VERSION}* ]]; then
  export PYENV_VERSION=$i
  echo $PYENV_VERSION
 fi
done
eval "$(pyenv init -)"
eval "$(pyenv virtualenv-init -)"
pyenv doctor
# Generate random string to avoid collisions durin testing
uuid=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 16 | head -n 1)
pyenv virtualenv-delete -f responses-$uuid 2>/dev/null || true
pyenv virtualenv $PYENV_VERSION responses-$uuid
pyenv activate responses-$uuid
curl https://bootstrap.pypa.io/get-pip.py -o /tmp/get-pip-${uuid}.py
python3 /tmp/get-pip-${uuid}.py
python3 -m pip install -r requirements.txt
python3 -m pip freeze
python3 main.py --private-key=example-test-key --password=PassPhrase -q=` + qName +
				` -r="amqp://` + rmqUser + `:` + rmqPassword + `@` + rmqHost + `:5672/%2f?connection_attempts=30&retry_delay=.5&socket_timeout=5" --output ` + outputFN + "\n"

			tmpl, errGo := textTemplate.New("python-response-queue").Parse(respCmd)
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			cmdLine := &bytes.Buffer{}
			if errGo = tmpl.Execute(cmdLine, nil); errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			script := filepath.Join(tmpDir, "response-capture.sh")
			if errGo := ioutil.WriteFile(script, cmdLine.Bytes(), 0700); errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			// done is a channel that will get closed once the asynchronous processing has completed
			done = make(chan struct{}, 1)

			// All the data is staged that needs to be so start the python process.  First
			// increment the wait group to  indicate we are using the response queue
			// and not to delete until this go function is done with it.
			logger.Debug("start python response watcher")
			dropResponse.Add(1)
			go func() {
				defer func() {
					dropResponse.Done()

					close(done)
				}()

				// Python run does everything including copying files etc to our temporary
				// directory.  The error is ignored because the script will exit when the queue
				// is deleted and raises and error.
				outputs, pyRunErr := runner.PythonRun(map[string]os.FileMode{}, tmpDir, script, 1024)
				defer func() {
					if pyRunErr != nil {
						outputs = append(outputs, fmt.Sprint("python run failed", "error", pyRunErr.Error()))
						select {
						case logsC <- outputs:
						default:
							logger.Info("python run failed", "error", err.Error())
						}
					}

					// If the canncel is closed we will get a panic so we should catch it.
					// In this case the close channel indicates the listener is no longer interested in logs
					// so we can ignore it and just print a warning
					defer func() {
						if r := recover(); r != nil {
							if len(outputs) != 0 {
								logger.Warn("python run logs were dropped", "outputs", strings.Join(outputs, "\n"))
							}
						}
					}()

					// Use the python run output as the default output unless
					// we can locate the real responses file in which case we use
					// that
					output, errGo := ioutil.ReadFile(outputFN)
					if errGo == nil {
						outputs = append(outputs, strings.Split(string(output), "\n")...)
					} else {
						logger.Warn("python run output not available", "error", err.Error())
					}

					select {
					case logsC <- outputs:
					default:
						logger.Info("unable to send python run log", "line", strings.Join(outputs, "\n"))
					}
				}()

			}()
		default:
			logger.Warn("no report handling style was selected")
		}
	}

	return done, nil
}
