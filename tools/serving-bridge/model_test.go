package main

// This file contains the implementations of various model index parsing and loading features

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/pkg/log"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/xid"
)

// eraseBucket is used to drop all of the objects in a bucket and then erase it once empty
func eraseBucket(ctx context.Context, s3Client *minio.Client, bucket string) (err kv.Error) {

	// Used by the remove function to receive object keys to be deleted that are
	// pumped into it by the ListObjects function
	objC := s3Client.ListObjects(ctx, bucket,
		minio.ListObjectsOptions{
			Recursive: true,
		})

	// Non blocking deletion that will signal its completion by closing the errorC channel
	// and will continue to process until the keyC channel is closed
	errorC := s3Client.RemoveObjects(ctx, bucket, objC, minio.RemoveObjectsOptions{})

	for e := range errorC {
		err = kv.Wrap(e.Err).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
		logger.Warn("remove object failed", "error", err.Error())
	}

	if errGo := s3Client.RemoveBucket(ctx, bucket); errGo != nil {
		return kv.Wrap(errGo).With("bucket", bucket).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// TestEmptyModelLoad will populate an S3 bucket with an empty index file and check that it loads
//
func TestEmptyModelLoad(t *testing.T) {
	logger := log.NewLogger("TestEmptyModelLoad")
	defer logger.Warn("TestEmptyModelLoad")

	// Check that the S3 test server has been started locally
	alive, err := WaitForMinioTest(context.Background())

	if err != nil {
		t.Fatal(err)
	}

	if !alive {
		err := kv.NewError("minio down").With("endpoint", *endpointOpt).With("stack", stack.Trace().TrimRuntime())
		t.Fatal(err)
	}

	// Create the test bucket and then place an empty index into it
	s3Client, errGo := minio.New(*endpointOpt, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKeyOpt, *secretKeyOpt, ""),
		Secure: false,
	})
	if errGo != nil {
		err := kv.Wrap(errGo).With("endpoint", *endpointOpt).With("stack", stack.Trace().TrimRuntime())
		t.Fatal(err)
	}

	bucket := *bucketOpt
	objsCreated := []minio.ObjectInfo{}

	// If the bucket does not exist then create it and have it destroyed on completion.  If the bucket
	// does exist then post a defer function that will just erase specific keys in the existing bucket.
	exists, errGo := s3Client.BucketExists(context.Background(), bucket)
	if errGo != nil {
		err := kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
		t.Fatal(err)
	}
	if !exists {
		if errGo = s3Client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); errGo != nil {
			err := kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
			t.Fatal(err)
		}
		defer func() {
			if err := eraseBucket(context.Background(), s3Client, bucket); err != nil {
				logger.Warn("Unable to cleanup test", "test", "TestEmptyModelLoad", "error", err.Error())
			}
		}()
	} else {
		// In the event we cannot delete the entire bucket as it already existed we will need to clean up artifacts
		// one by one and this is where we do this
		defer func() {
			objC := make(chan minio.ObjectInfo, 6)

			go func() {
				defer close(objC)
				for _, obj := range objsCreated {
					objC <- obj
				}
			}()
			for result := range s3Client.RemoveObjects(context.Background(), bucket, objC, minio.RemoveObjectsOptions{}) {
				err := kv.Wrap(result.Err).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
				logger.Warn("Unable to cleanup test", "test", "TestEmptyModelLoad", "error", err.Error())
			}
		}()
	}

	// Now create an empty index file
	key := indexPrefix + xid.New().String() + indexSuffix
	buffer := []byte{'Z'}
	uploadInfo, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader(buffer), int64(len(buffer)),
		minio.PutObjectOptions{
			ContentType: "application/octet-stream",
		})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	// Get the ObjectInfo for the new blob and add it to the cleanup list
	objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	objsCreated = append(objsCreated, objInfo)

	logger.Info("Debug", "", spew.Sdump(uploadInfo), "stack", stack.Trace().TrimRuntime())
	until := time.Now().Add(time.Minute)

	for {
		// Wait for the server to complete an update pass
		WaitForScan(context.Background())

		// Now examine the server state for the presence of the index file with no blobs

		knownIndexes.Lock()
		mdl, isPresent := knownIndexes.models[key]
		knownIndexes.Unlock()

		if isPresent {
			logger.Debug("Test results", "mdl", mdl)
		}

		if time.Now().After(until) {
			t.Fatal(kv.NewError("model not loaded").With("endpoint", *endpointOpt, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime()))
		}
	}
}
