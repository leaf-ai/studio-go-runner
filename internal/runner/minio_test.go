// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/leaf-ai/go-service/pkg/log"
	minio_local "github.com/leaf-ai/go-service/pkg/minio"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/s3"

	minio "github.com/minio/minio-go/v7"
	"github.com/rs/xid"

	"github.com/davecgh/go-spew/spew" // MIT License
)

// This file contains tests related to accessing and using the S3 APIs
// used by the runner

func s3AnonAccess(t *testing.T, mts *minio_local.MinioTestServer, logger *log.Logger) {

	// Check that the minio local server has initialized before continuing
	ctx := context.Background()

	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	if alive, err := mts.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("The minio test server is not available to run this test", mts.Address)
	}
	logger.Info("Alive checked", "addr", mts.Address)

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
			mts.RemoveBucketAll(item.bucket)
		}
	}()

	for _, item := range bucketsAndFiles {
		for _, blob := range item.blobs {
			if err := mts.UploadTestFile(item.bucket, blob.key, (int64)(blob.size)); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Make the last bucket public and its contents
	if err := mts.SetPublic(bucketsAndFiles[len(bucketsAndFiles)-1].bucket); err != nil {
		t.Fatal(err)
	}

	// access using both secured and unsecured the buckets we have to validate access
	creds := request.AWSCredential{
		AccessKey: mts.AccessKeyId,
		SecretKey: mts.SecretAccessKeyId,
	}
	env := map[string]string{}

	for i, item := range bucketsAndFiles {
		authS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, item.bucket, "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, blob := range item.blobs {
			if _, err = authS3.Hash(ctx, blob.key); err != nil {
				t.Fatal(err)
			}
		}
		authS3.Close()

		creds.AccessKey = ""
		creds.SecretKey = ""
		// The last bucket is the one with the anonymous access
		if i == len(bucketsAndFiles)-1 {
			anonS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, item.bucket, "", false, false)
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
			anonS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, item.bucket, "", false, false)
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

// TestS3Anon will test anonymous access to S3 public resources using Minio
//
func TestS3MinioAnon(t *testing.T) {

	logger := log.NewLogger("s3_anon_access")

	mts, _ := minio_local.InitTestingMinio(context.Background(), false)

	s3AnonAccess(t, mts, logger)
}
