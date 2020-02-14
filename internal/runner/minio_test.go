// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/jjeffery/kv" // MIT License
	"github.com/leaf-ai/studio-go-runner/pkg/studio"
	minio "github.com/minio/minio-go"
	"github.com/rs/xid"
)

// This file contains tests related to accessing and using the S3 APIs
// used by the runner

func s3AnonAccess(t *testing.T, logger *studio.Logger) {

	// Check that the minio local server has initialized before continuing
	ctx := context.Background()

	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	if alive, err := MinioTest.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("The minio test server is not available to run this test", MinioTest.Address)
	}
	logger.Info("Alive checked", "addr", MinioTest.Address)

	type blob struct {
		key  string
		size int
	}

	type testData struct {
		bucket string
		blobs  []blob
	}

	// Create multiple buckets and upload to all
	bucketsAndFiles := []testData{
		{xid.New().String(), []blob{{xid.New().String(), rand.Intn(8192)}}},
		{xid.New().String(), []blob{{xid.New().String(), rand.Intn(8192)}}},
	}

	// Cleanup after ourselves as best as we can on the remote minio server
	defer func() {
		for _, item := range bucketsAndFiles {
			MinioTest.RemoveBucketAll(item.bucket)
		}
	}()

	for _, item := range bucketsAndFiles {
		for _, blob := range item.blobs {
			if err := MinioTest.UploadTestFile(item.bucket, blob.key, (int64)(blob.size)); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Make the last bucket public and its contents
	if err := MinioTest.SetPublic(bucketsAndFiles[len(bucketsAndFiles)-1].bucket); err != nil {
		t.Fatal(err)
	}

	// access using both secured and unsecured the buckets we have to validate access
	env := map[string]string{
		"MINIO_ACCESS_KEY":  MinioTest.AccessKeyId,
		"MINIO_SECRET_KEY":  MinioTest.SecretAccessKeyId,
		"MINIO_TEST_SERVER": MinioTest.Address,
	}
	creds := ""

	for i, item := range bucketsAndFiles {
		authS3, err := NewS3storage(ctx, "testProject", creds, env, MinioTest.Address, item.bucket, "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, blob := range item.blobs {
			if _, err = authS3.Hash(ctx, blob.key); err != nil {
				t.Fatal(err)
			}
		}
		authS3.Close()

		// The last bucket is the one with the anonymous access
		if i == len(bucketsAndFiles)-1 {
			anonS3, err := NewS3storage(ctx, "testProject", creds, map[string]string{"MINIO_TEST_SERVER": MinioTest.Address}, "", item.bucket, "", false, false)
			if err != nil {
				t.Fatal(err)
			}
			for _, blob := range item.blobs {
				if _, err = anonS3.Hash(ctx, blob.key); err != nil {
					t.Fatal(err)
				}
			}
			anonS3.Close()
		}

		// Take the first bucket and make sure we cannot access it and get an error of some description as a negative test
		if i == 0 {
			anonS3, err := NewS3storage(ctx, "testProject", creds, map[string]string{"MINIO_TEST_SERVER": MinioTest.Address}, "", item.bucket, "", false, false)
			if err != nil {
				continue
			}
			for _, blob := range item.blobs {
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

func reportErrors(ctx context.Context, errorC <-chan kv.Error, logger *studio.Logger) {
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-errorC:
			if err == nil {
				continue
			}
			logger.Warn(err.Error())
		}
	}
}

// TestS3Anon will test anonymous access to S3 public resources using Minio
//
func TestS3MinioAnon(t *testing.T) {

	logger := studio.NewLogger("s3_anon_access")

	InitTestingMinio(context.Background(), false)

	s3AnonAccess(t, logger)
}
