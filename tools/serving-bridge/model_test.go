package main

// This file contains the implementations of various model index parsing and loading features

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
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
			UseV1:        true,
			Recursive:    true,
			WithMetadata: true,
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

// TestModelLoad will populate an S3 bucket with an empty index file and check that it loads
//
func TestModelLoad(t *testing.T) {
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
		logger.Trace("Making bucket", bucket)
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
		logger.Trace("Using existing the bucket", bucket)
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

	// Run model index creation multiple times with increasing numbers of components
	for i := 0; i != 4; i++ {

		// Used by the index file later
		payload := strings.Builder{}

		for aBlob := 0; aBlob != i; aBlob++ {
			key := xid.New().String() + ".dat"
			data := runner.RandomString(rand.Intn(8192-4096) + 4096)
			uploadInfo, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(data)), int64(len(data)),
				minio.PutObjectOptions{})
			if errGo != nil {
				t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
			}
			// Get the ObjectInfo for the new blob and add it to the cleanup list
			objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
			if errGo != nil {
				t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
			}

			objsCreated = append(objsCreated, objInfo)

			payload.WriteString(fmt.Sprintf("%s,%s\n", key, uploadInfo.ETag))
		}

		// Now create an empty index file
		key := indexPrefix + xid.New().String() + indexSuffix
		uploadInfo, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
			minio.PutObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		// Get the ObjectInfo for the new blob and add it to the cleanup list
		objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", *endpointOpt, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		objsCreated = append(objsCreated, objInfo)

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		mdl, err := waitForIndex(ctx, s3Client.EndpointURL().String(), bucket, key)
		if err != nil {
			t.Fatal(err)
		}

		if i != len(mdl.blobs) {
			logger.Debug("Test results", "mdl", spew.Sdump(mdl), "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model loaded too many items").With("endpoint", *endpointOpt, "bucket", bucket, "key", key, "expected blobs", i, "actual blobs", len(mdl.blobs)).With("stack", stack.Trace().TrimRuntime()))
		}
		if mdl.obj == nil {
			logger.Debug("Test results", "mdl", spew.Sdump(mdl), "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model info missing").With("endpoint", *endpointOpt, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime()))
		}
		if strings.Trim(uploadInfo.ETag, "\"") != strings.Trim(mdl.obj.ETag, "\"") {
			logger.Debug("Test results", "mdlETag", mdl.obj.ETag, "uploaded ETag", uploadInfo.ETag, "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model ETag unexpected").With("endpoint", *endpointOpt, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime()))
		}

		logger.Debug("Model index tested", "components", i, "stack", stack.Trace().TrimRuntime())
	}
}

func waitForIndex(ctx context.Context, endpoint string, bucket string, key string) (mdl *model, err kv.Error) {
	for {
		// Wait for the server to complete an update pass
		WaitForScan(ctx)

		select {
		case <-ctx.Done():
			return nil, kv.NewError("model load stopped").With("endpoint", *endpointOpt, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
		default:
		}

		// Now examine the server state for the presence of the index file with no blobs
		knownIndexes.Lock()
		mdl, isPresent := knownIndexes.models[endpoint][key]
		knownIndexes.Unlock()

		if isPresent {
			return mdl, nil
		}
	}
}
