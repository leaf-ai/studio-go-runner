// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/go-service/pkg/log"
	minio_local "github.com/leaf-ai/go-service/pkg/minio"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/s3"
	"github.com/tebeka/atexit"

	minio "github.com/minio/minio-go/v7"
	"github.com/rs/xid"

	"github.com/davecgh/go-spew/spew" // MIT License
)

// This file contains tests related to accessing and using the S3 APIs
// used by the runner

var (
	initMinio  sync.Once
	mts        *minio_local.MinioTestServer
	minioGuard sync.Mutex
)

func startMinio() {
	minioGuard.Lock()
	defer minioGuard.Unlock()

	mtsCtx, mtsCancel := context.WithCancel(context.Background())
	mts, _ = minio_local.InitTestingMinio(mtsCtx, false)

	atexit.Register(mtsCancel)

}

func s3AliveCheck(ctx context.Context, t *testing.T, mts *minio_local.MinioTestServer) {
	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	if alive, err := mts.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("The minio test server is not available to run this test", mts.Address)
	}
}

type blob struct {
	key  string
	size int
}

type testData struct {
	name  string
	blobs []blob
}

func s3TestFiles(ctx context.Context, mts *minio_local.MinioTestServer, buckets int, blobs int) (bucketsAndFiles []testData, err kv.Error) {

	// Create multiple buckets and upload to all
	bucketsAndFiles = make([]testData, buckets, buckets)
	for i := 0; i != buckets; i++ {
		bucketsAndFiles[i] = testData{
			name:  xid.New().String(),
			blobs: make([]blob, blobs, blobs),
		}
		for j := 0; j != blobs; j++ {
			bucketsAndFiles[i].blobs[j] = blob{
				key:  xid.New().String(),
				size: rand.Intn(8192-1024+1) + 1024,
			}
		}
	}

	// Cleanup after ourselves as best as we can on the remote minio server
	go func() {
		_ = <-ctx.Done()
		for _, bucket := range bucketsAndFiles {
			mts.RemoveBucketAll(bucket.name)
		}
	}()

	for _, bucket := range bucketsAndFiles {
		for _, blob := range bucket.blobs {
			if err := mts.UploadTestFile(bucket.name, blob.key, (int64)(blob.size)); err != nil {
				return bucketsAndFiles, err
			}
		}
	}
	return bucketsAndFiles, nil
}

func s3AnonAccess(t *testing.T, mts *minio_local.MinioTestServer, logger *log.Logger) {

	// Check that the minio local server has initialized before continuing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s3AliveCheck(ctx, t, mts)
	if t.Failed() {
		return
	}

	logger.Info("Alive checked", "addr", mts.Address)

	bucketsAndFiles, err := s3TestFiles(ctx, mts, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug("stack", stack.Trace().TrimRuntime())
	// Make the last bucket public and its contents
	if err := mts.SetPublic(bucketsAndFiles[len(bucketsAndFiles)-1].name); err != nil {
		t.Fatal(err)
	}

	// access using both secured and unsecured the buckets we have to validate access
	creds := request.AWSCredential{
		AccessKey: mts.AccessKeyId,
		SecretKey: mts.SecretAccessKeyId,
	}
	env := map[string]string{}

	for i, bucket := range bucketsAndFiles {
		authS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, bucket.name, "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, blob := range bucket.blobs {
			if _, err = authS3.Hash(ctx, blob.key); err != nil {
				t.Fatal(err)
			}
		}
		authS3.Close()

		creds.AccessKey = ""
		creds.SecretKey = ""
		// The last bucket is the one with the anonymous access
		if i == len(bucketsAndFiles)-1 {
			anonS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, bucket.name, "", false, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, blob := range bucket.blobs {
				if _, err = anonS3.Hash(ctx, blob.key); err != nil {
					t.Fatal(err)
				}
			}
			anonS3.Close()
		}

		// Take the first bucket and make sure we cannot access it and get an error of some description as a negative test
		if i == 0 {
			anonS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, bucket.name, "", false, false)
			if err != nil {
				continue
			}
			for _, blob := range bucket.blobs {
				if _, err = anonS3.Hash(ctx, blob.key); err != nil {
					if unwrap, ok := err.(interface{ Unwrap() error }); ok {
						if minio.ToErrorResponse(unwrap.Unwrap()).Code == "AccessDenied" {
							continue
						}
					}
					t.Fatal("A private bucket when accessed using anonymous credentials raise an unexpected type of error", spew.Sdump(err))
				} else {
					t.Fatal("A private bucket when accessed using anonymous credentials did not raise an error")
				}
			}
			anonS3.Close()
		}
	}
}

func s3Limiter(t *testing.T, mts *minio_local.MinioTestServer, logger *log.Logger) {
	// Check that the minio local server has initialized before continuing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s3AliveCheck(ctx, t, mts)
	if t.Failed() {
		return
	}

	// Upload several files to the S3 servera
	bucketsAndFiles, err := s3TestFiles(ctx, mts, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Use the Storage API to connect to the server
	creds := request.AWSCredential{
		AccessKey: mts.AccessKeyId,
		SecretKey: mts.SecretAccessKeyId,
	}
	env := map[string]string{}

	// Use the fetch calls to test limitations
	for _, bucket := range bucketsAndFiles {
		authS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, bucket.name, "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, blob := range bucket.blobs {
			if _, err = authS3.Hash(ctx, blob.key); err != nil {
				t.Fatal(err)
			}
		}
		authS3.Close()
	}
}

// TestS3Anon will test anonymous access to S3 public resources using Minio
//
func TestS3MinioAnon(t *testing.T) {

	logger := log.NewLogger("s3_anon_access")

	initMinio.Do(startMinio)

	logger.Debug("stack", stack.Trace().TrimRuntime())
	s3AnonAccess(t, mts, logger)
	logger.Debug("stack", stack.Trace().TrimRuntime())
}

// TestS3Limiter is used to test that the storage APIs inside the runner can successfully limit
// file system comsumption during downloads
//
func TestS3Limiter(t *testing.T) {
	logger := log.NewLogger("s3_limiter")

	initMinio.Do(startMinio)

	s3Limiter(t, mts, logger)
}
