package runner

// This file contains a skeleton wrapper for running a minio
// server in-situ and is principally useful for when testing
// is being done and a mocked S3 is needed, this case
// we provide a full implementaiton as minio offers a full
// implementation

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	minio "github.com/minio/minio-go"
	"github.com/rs/xid" // MIT
)

type MinioTestServer struct {
	AccessKeyId       string
	SecretAccessKeyId string
	Address           string
	Client            *minio.Client
}

type MinioCfgJson struct {
	Version    string `json:"version"`
	Credential struct {
		AccessKey string `json:"accessKey"`
		SecretKey string `json:"secretKey"`
	} `json:"credential"`
	Region       string `json:"region"`
	Browser      string `json:"browser"`
	Worm         string `json:"worm"`
	Domain       string `json:"domain"`
	Storageclass struct {
		Standard string `json:"standard"`
		Rrs      string `json:"rrs"`
	} `json:"storageclass"`
	Cache struct {
		Drives  []interface{} `json:"drives"`
		Expiry  int           `json:"expiry"`
		Maxuse  int           `json:"maxuse"`
		Exclude []interface{} `json:"exclude"`
	} `json:"cache"`
}

var (
	MinioTest = &MinioTestServer{
		AccessKeyId:       xid.New().String(),
		SecretAccessKeyId: xid.New().String(),
		Client:            nil,
	}
)

func (mts *MinioTestServer) Upload(bucket string, key string, file string) (err errors.Error) {

	f, errGo := os.Open(file)
	if errGo != nil {
		return errors.Wrap(errGo, "Upload passed a non-existant file name").With("file", file).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()

	exists, errGo := mts.Client.BucketExists(bucket)
	if errGo != nil {
		return errors.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}
	if !exists {
		if errGo = mts.Client.MakeBucket(bucket, ""); errGo != nil {
			return errors.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
		}
	}

	_, errGo = mts.Client.PutObject(bucket, key, bufio.NewReader(f), -1,
		minio.PutObjectOptions{
			ContentType:  "application/octet-stream",
			CacheControl: "max-age=600",
		})

	if errGo != nil {
		return errors.Wrap(errGo).With("bucket", bucket).With("key", key).With("file", file).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func writeCfg(mts *MinioTestServer) (cfgDir string, err errors.Error) {
	// Initialize a configuration directory for the minio server
	// complete with the json configuration containing the credentials
	// for the test server
	cfgDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	cfg := MinioCfgJson{}
	cfg.Version = "26"
	cfg.Credential.AccessKey = mts.AccessKeyId
	cfg.Credential.SecretKey = mts.SecretAccessKeyId

	result, errGo := json.MarshalIndent(cfg, "", "    ")
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if errGo = ioutil.WriteFile(path.Join(cfgDir, "config.json"), result, 0666); errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return cfgDir, nil
}

// startMinio will fork off a running minio server with an empty data store
// that can be used for testing purposes.  This function does not block,
// however it does start a go routine
//
func startMinio(ctx context.Context, errC chan errors.Error) (tmpDir string, err errors.Error) {

	// First check that the minio executable is present on the test system
	//
	// We are using the executable because the dependency heirarchy of minio
	// is very tangled and so it is very hard to embeed for now, Go 1.10.3
	execPath, errGo := exec.LookPath("minio")
	if errGo != nil {
		return tmpDir, errors.Wrap(errGo, "please install minio into your path").With("stack", stack.Trace().TrimRuntime())
	}

	// Get a free server listening port for our test
	port, err := GetFreePort("127.0.0.1:0")
	if err != nil {
		return tmpDir, err
	}
	MinioTest.Address = fmt.Sprintf("127.0.0.1:%d", port)
	// Initialize the data directory for the file server
	tmpDir, errGo = ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return "", errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if errGo = os.Chmod(tmpDir, 0777); errGo != nil {
		return "", errors.Wrap(errGo).With("tmpDir", tmpDir).With("stack", stack.Trace().TrimRuntime())
	}

	cfgDir, err := writeCfg(MinioTest)
	if err != nil {
		return tmpDir, err
	}

	cmdCtx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(cmdCtx, execPath,
		"server",
		"--address", MinioTest.Address,
		"--config-dir", cfgDir,
		tmpDir,
	)

	go func() {
		// When the main process stops kill our cmd runner for minio
		go func() {
			<-ctx.Done()
			cancel()

		}()
		stdout, errGo := cmd.StdoutPipe()
		if errGo != nil {
			errC <- errors.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
		}
		stderr, errGo := cmd.StderrPipe()
		if errGo != nil {
			errC <- errors.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
		}
		// Non-blockingly echo command output to terminal
		go io.Copy(os.Stdout, stdout)
		go io.Copy(os.Stderr, stderr)

		if errGo = cmd.Start(); errGo != nil {
			errC <- errors.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
		}

		if errGo = cmd.Wait(); errGo != nil {
			errC <- errors.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
		}

		defer os.RemoveAll(tmpDir)
		defer os.RemoveAll(cfgDir)
	}()

	// TODO Wait for the server to start
	time.Sleep(3 * time.Second)

	if MinioTest.Client == nil {
		client, errGo := minio.New(MinioTest.Address, MinioTest.AccessKeyId,
			MinioTest.SecretAccessKeyId, false)
		if errGo != nil {
			return tmpDir, errors.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
		}

		MinioTest.Client = client
	}

	return tmpDir, nil
}

// LocalMinio will fork a minio server that can he used for staging and test
// in a manner that also wraps an error reporting channel and a means of
// stopping it
//
func LocalMinio(ctx context.Context) (errC chan errors.Error) {
	errC = make(chan errors.Error, 5)

	// Do much for the work upfront so that we know that the address
	// of our test S3 server is running prior to the caller
	// continuing
	tmpDir, err := startMinio(ctx, errC)

	go func(ctx context.Context) {

		func() {
			for {
				select {
				case errC <- err:
				case <-ctx.Done():
					// TODO: Determine how the minio server might be able to be stopped
					// and implement that here.  It is not currently supported by the API
					// however deleting the folders then requesting a file or something
					// similar might be able to be done
					return
				default:
				}
			}
		}()

		if len(tmpDir) != 0 {
			os.RemoveAll(tmpDir)
		}
	}(ctx)

	return errC
}
