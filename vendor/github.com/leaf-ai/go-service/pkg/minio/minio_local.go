// Copyright 2018-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package minio_local // import "github.com/leaf-ai/go-service/pkg/minio_local"

// This file contains a skeleton wrapper for running a minio
// server in-situ and is principally useful for when testing
// is being done and a mocked S3 is needed, in this case
// we provide a full implementation as minio offers a full
// implementation

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/leaf-ai/go-service/pkg/network"

	"go.uber.org/atomic"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/rs/xid" // MIT
)

// MinioTestServer encapsulates all of the data needed to run
// a test minio server instance
//
type MinioTestServer struct {
	AccessKeyId       string
	SecretAccessKeyId string
	Address           string
	Client            *minio.Client
	Ready             atomic.Bool
	ProcessState      *os.ProcessState
}

func NewMinioTest() (minioTest *MinioTestServer) {
	minioTest = &MinioTestServer{
		AccessKeyId:       xid.New().String(),
		SecretAccessKeyId: xid.New().String(),
	}

	minioTest.Ready.Store(false)

	return minioTest
}

// MinioCfgJson stores configuration information to be written to a disk based configuration
// file prior to starting a test minio instance
//
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
	minioAccessKey  = flag.String("minio-access-key", "", "Specifies an AWS access key for a minio server used during testing, accepts ${} env var expansion")
	minioSecretKey  = flag.String("minio-secret-key", "", "Specifies an AWS secret access key for a minio server used during testing, accepts ${} env var expansion")
	minioTestServer = flag.String("minio-test-server", "", "Specifies an existing minio server that is available for testing purposes, accepts ${} env var expansion")
)

// TmpDirFile creates a temporary file of a given size and passes back the directory it
// was generated in along with its name
func TmpDirFile(size int64) (dir string, fn string, err kv.Error) {

	tmpDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return "", "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	fn = path.Join(tmpDir, xid.New().String())
	f, errGo := os.Create(fn)
	if errGo != nil {
		return "", "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer func() { _ = f.Close() }()

	if errGo = f.Truncate(size); errGo != nil {
		return "", "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return tmpDir, fn, nil
}

// UploadTestFile will create and upload a file of a given size to the MinioTest server to
// allow test cases to exercise functionality based on S3
//
func (mts *MinioTestServer) UploadTestFile(bucket string, key string, size int64) (err kv.Error) {
	tmpDir, fn, err := TmpDirFile(size)
	if err != nil {
		return err
	}
	defer func() {
		if errGo := os.RemoveAll(tmpDir); errGo != nil {
			fmt.Printf("%s %#v", tmpDir, errGo)
		}
	}()

	// Get the Minio Test Server instance and sent it some random data while generating
	// a hash
	return mts.Upload(bucket, key, fn)
}

// SetPublic can be used to enable public access to a bucket
//
func (mts *MinioTestServer) SetPublic(bucket string) (err kv.Error) {
	if !mts.Ready.Load() {
		return kv.NewError("server not ready").With("host", mts.Address).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}
	policy := `{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Action": [
        "s3:GetObject"
      ],
      "Effect": "Allow",
      "Principal": {
        "AWS": [
          "*"
        ]
      },
      "Resource": [
        "arn:aws:s3:::%s/*"
      ],
      "Sid": ""
    }
  ]
}`

	if errGo := mts.Client.SetBucketPolicy(context.Background(), bucket, fmt.Sprintf(policy, bucket)); errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// RemoveBucketAll empties the identified bucket on the minio test server
// identified by the mtx receiver variable
//
func (mts *MinioTestServer) RemoveBucketAll(bucket string) (errs []kv.Error) {

	if !mts.Ready.Load() {
		errs = append(errs, kv.NewError("server not ready").With("host", mts.Address).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		return errs
	}

	exists, errGo := mts.Client.BucketExists(context.Background(), bucket)
	if errGo != nil {
		errs = append(errs, kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		return errs
	}
	if !exists {
		return nil
	}

	doneC := make(chan struct{})
	defer close(doneC)

	// This channel is used to send keys on that will be deleted in the background.
	// We dont yet have large buckets that need deleting so the asynchronous
	// features of this are not used but they very well could be used in the future.
	keysC := make(chan minio.ObjectInfo)
	errLock := sync.Mutex{}

	// Send object names that are needed to be removed though a worker style channel
	// that might be a little slower, but for our case with small buckets is not
	// yet an issue so leave things as they are
	go func() {
		defer close(keysC)

		listCtx, listCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer listCancel()

		// List all objects from a bucket-name with a matching prefix.
		for object := range mts.Client.ListObjects(listCtx, bucket, minio.ListObjectsOptions{
			Recursive: true,
			UseV1:     true,
		}) {
			if object.Err != nil {
				errLock.Lock()
				errs = append(errs, kv.Wrap(object.Err).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
				errLock.Unlock()
				continue
			}
			select {
			case keysC <- object:
			case <-time.After(2 * time.Second):
				errLock.Lock()
				errs = append(errs, kv.NewError("object delete timeout").With("key", object.Key).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
				errLock.Unlock()
				// Giveup deleting an object if it blocks everything
			}
		}
	}()

	rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer rmCancel()
	for errMinio := range mts.Client.RemoveObjects(rmCtx, bucket, keysC, minio.RemoveObjectsOptions{}) {
		if errMinio.Err.Error() == "EOF" {
			break
		}
		errLock.Lock()
		errs = append(errs, kv.NewError(errMinio.Err.Error()).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		errLock.Unlock()
	}

	errGo = mts.Client.RemoveBucket(rmCtx, bucket)
	if errGo != nil {
		errs = append(errs, kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
	}
	return errs
}

// Upload will take the nominated file, file parameter, and will upload it to the bucket and key
// pair on the server identified by the mtx receiver variable
//
func (mts *MinioTestServer) Upload(bucket string, key string, file string) (err kv.Error) {

	if !mts.Ready.Load() {
		return kv.NewError("server not ready").With("host", mts.Address).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}

	f, errGo := os.Open(filepath.Clean(file))
	if errGo != nil {
		return kv.Wrap(errGo, "Upload passed a non-existent file name").With("file", file).With("stack", stack.Trace().TrimRuntime())
	}
	defer f.Close()

	exists, errGo := mts.Client.BucketExists(context.Background(), bucket)
	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}
	if !exists {
		if errGo = mts.Client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); errGo != nil {
			return kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	_, errGo = mts.Client.PutObject(ctx, bucket, key, bufio.NewReader(f), -1,
		minio.PutObjectOptions{
			ContentType:  "application/octet-stream",
			CacheControl: "max-age=600",
		})

	if errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket).With("key", key).With("file", file).With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

func (mts *MinioTestServer) writeCfg() (cfgDir string, err kv.Error) {
	// Initialize a configuration directory for the minio server
	// complete with the json configuration containing the credentials
	// for the test server
	cfgDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	cfg := MinioCfgJson{}
	cfg.Version = "26"
	cfg.Credential.AccessKey = mts.AccessKeyId
	cfg.Credential.SecretKey = mts.SecretAccessKeyId
	cfg.Worm = "off"

	result, errGo := json.MarshalIndent(cfg, "", "    ")
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if errGo = ioutil.WriteFile(path.Join(cfgDir, "config.json"), result, 0600); errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return cfgDir, nil
}

// startLocalMinio will fork off a running minio server with an empty data store
// that can be used for testing purposes.  This function does not block,
// however it does start a go routine
//
func (mts *MinioTestServer) startLocalMinio(ctx context.Context, retainWorkingDirs bool, errC chan kv.Error) {

	// Default to the case that another pod for external host has a running minio server for us
	// to use during testing
	if len(*minioTestServer) != 0 {
		mts.Address = os.ExpandEnv(*minioTestServer)
	}
	if len(*minioAccessKey) != 0 {
		mts.AccessKeyId = os.ExpandEnv(*minioAccessKey)
	}
	if len(*minioSecretKey) != 0 {
		mts.SecretAccessKeyId = os.ExpandEnv(*minioSecretKey)
	}

	// If we dont have a k8s based minio server specified for our test try try using a local
	// minio instance within the container or machine the test is run on
	//
	if len(*minioTestServer) == 0 {
		// First check that the minio executable is present on the test system
		//
		// We are using the executable because the dependency hierarchy of minio
		// is very tangled and so it is very hard to embeed for now, Go 1.10.3
		execPath, errGo := exec.LookPath("minio")
		if errGo != nil {
			errC <- kv.Wrap(errGo, "please install minio into your path").With("path", os.Getenv("PATH")).With("stack", stack.Trace().TrimRuntime())
			return
		}

		// Get a free server listening port for our test
		port, err := network.GetFreePort("127.0.0.1:0")
		if err != nil {
			errC <- err
			return
		}

		mts.Address = fmt.Sprintf("127.0.0.1:%d", port)

		// Initialize the data directory for the file server
		storageDir, errGo := ioutil.TempDir("", xid.New().String())
		if errGo != nil {
			errC <- kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			return
		}
		filepath.Clean(storageDir)

		if errGo = os.Chmod(storageDir, 0700); errGo != nil {
			errC <- kv.Wrap(errGo).With("storageDir", storageDir).With("stack", stack.Trace().TrimRuntime())
			os.RemoveAll(storageDir)
			return
		}

		// If we see no credentials were supplied for a local test, the typical case
		// then supply some defaults
		if len(mts.AccessKeyId) == 0 {
			mts.AccessKeyId = "UserUser"
		}
		if len(mts.SecretAccessKeyId) == 0 {
			mts.SecretAccessKeyId = "PasswordPassword"
		}

		// Now write a cfg file out for our desired minio
		// configuration
		cfgDir, err := mts.writeCfg()
		if err != nil {
			errC <- err
			return
		}
		cfgDir = filepath.Clean(cfgDir)

		go func() {

			// #nosec
			cmd := exec.CommandContext(ctx, filepath.Clean(execPath),
				"server",
				"--address", mts.Address,
				"--config-dir", cfgDir,
				storageDir,
			)

			stdout, errGo := cmd.StdoutPipe()
			if errGo != nil {
				errC <- kv.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
			}
			stderr, errGo := cmd.StderrPipe()
			if errGo != nil {
				errC <- kv.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
			}
			// Non-blockingly echo command output to terminal
			go io.Copy(os.Stdout, stdout)
			go io.Copy(os.Stderr, stderr)

			if errGo = cmd.Start(); errGo != nil {
				errC <- kv.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
			}

			if errGo = cmd.Wait(); errGo != nil {
				if errGo.Error() != "signal: killed" {
					errC <- kv.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
				}
			}

			// Give the test system access to the process details in which the minio was run
			mts.ProcessState = cmd.ProcessState

			if cmd.ProcessState.Exited() {
				fmt.Printf("%v\n", kv.NewError("minio self terminated").With("cfgDir", cfgDir, "storageDir", storageDir).With("stack", stack.Trace().TrimRuntime()))
			} else {
				if errCmd := cmd.Process.Kill(); errCmd == nil {
					fmt.Printf("%v\n", kv.NewError("minio force terminated").With("cfgDir", cfgDir, "storageDir", storageDir).With("stack", stack.Trace().TrimRuntime()))
				} else {
					fmt.Printf("%v\n", kv.Wrap(errCmd).With("cfgDir", cfgDir, "storageDir", storageDir).With("stack", stack.Trace().TrimRuntime()))
				}
			}

			if !retainWorkingDirs {
				os.RemoveAll(storageDir)
				os.RemoveAll(cfgDir)
			}
		}()
	}

	mts.startMinioClient(ctx, errC)
}

func (mts *MinioTestServer) startMinioClient(ctx context.Context, errC chan kv.Error) {
	// Wait for the server to start by checking the listen port using
	// TCP
	check := time.NewTicker(time.Second)
	defer check.Stop()

	for {
		select {
		case <-check.C:
			client, errGo := minio.New(mts.Address, &minio.Options{
				Creds:  credentials.NewStaticV4(mts.AccessKeyId, mts.SecretAccessKeyId, ""),
				Secure: false,
			})
			if errGo != nil {
				errC <- kv.Wrap(errGo, "minio failed").With("stack", stack.Trace().TrimRuntime())
				continue
			}
			mts.Client = client
			mts.Ready.Store(true)
			return
		case <-ctx.Done():
			return
		}
	}
}

// IsAlive is used to test if the expected minio local test server is alive
//
func (mts *MinioTestServer) IsAlive(ctx context.Context) (alive bool, err kv.Error) {

	if mts == nil {
		return false, nil
	}

	check := time.NewTicker(5 * time.Second)
	defer check.Stop()

	for {
		select {
		case <-ctx.Done():
			return false, err
		case <-check.C:
			if !mts.Ready.Load() || mts.Client == nil {
				continue
			}
			_, errGo := mts.Client.BucketExists(context.Background(), xid.New().String())
			if errGo == nil {
				return true, nil
			}
			err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
}

// InitTestingMinio will fork a minio server that can he used for staging and test
// in a manner that also wraps an error reporting channel and a means of
// stopping it
//
func InitTestingMinio(ctx context.Context, retainWorkingDirs bool) (mts *MinioTestServer, errC chan kv.Error) {
	errC = make(chan kv.Error, 5)

	mts = NewMinioTest()

	mts.startLocalMinio(ctx, retainWorkingDirs, errC)

	return mts, errC
}
