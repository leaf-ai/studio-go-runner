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

type cleanUpFunc func(s3Client *minio.Client, bucket string, objsCreated []minio.ObjectInfo)

// initTestBucket is used to initialize the test bucket and then also supply the right cleanup function
// as a returned function callback that can be used by the test to do the appropriate cleanup
// actions.
func initTestBucket(s3Client *minio.Client, endpoint string, bucket string) (cleanUp cleanUpFunc, err kv.Error) {
	// If the bucket does not exist then create it and have it destroyed on completion.  If the bucket
	// does exist then post a defer function that will just erase specific keys in the existing bucket.
	exists, errGo := s3Client.BucketExists(context.Background(), bucket)
	if errGo != nil {
		err := kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
		return nil, err
	}
	if !exists {
		logger.Trace("Making bucket", bucket)
		if errGo = s3Client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{}); errGo != nil {
			err := kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
			return nil, err
		}
		return func(s3Client *minio.Client, bucket string, objsCreated []minio.ObjectInfo) {
			if err := eraseBucket(context.Background(), s3Client, bucket); err != nil {
				logger.Warn("Unable to cleanup test", "test", "TestEmptyModelLoad", "error", err.Error())
			}
		}, nil
	}
	logger.Trace("Using existing the bucket", bucket)

	// In the event we cannot delete the entire bucket as it already existed we will need to clean up artifacts
	// one by one and this is where we do this
	return func(s3Client *minio.Client, bucket string, objsCreated []minio.ObjectInfo) {
		objC := make(chan minio.ObjectInfo, 6)

		go func() {
			defer close(objC)
			for _, obj := range objsCreated {
				objC <- obj
			}
		}()
		for result := range s3Client.RemoveObjects(context.Background(), bucket, objC, minio.RemoveObjectsOptions{}) {
			err := kv.Wrap(result.Err).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime())
			logger.Warn("Unable to cleanup test", "test", "TestEmptyModelLoad", "error", err.Error())
		}
	}, nil
}

// initTestWithMinio waits for the in situ minio server to start and will then load appropriate test
// parameters for use with the server, generate or reuse an existing bucket and then return both a live
// minio client for the server and an appropriate callback function for cleaning up the servers
// resources
//
func initTestWithMinio(endpoint string, bucket string) (s3Client *minio.Client, detectedEndpoint string, cleanUp cleanUpFunc, err kv.Error) {
	// Check that the S3 test server has been started locally
	alive, err := WaitForMinioTest(context.Background())

	if err != nil {
		return nil, endpoint, nil, err
	}

	if len(endpoint) == 0 {
		endpoint = *endpointOpt
	}

	if !alive {
		return nil, endpoint, nil, kv.NewError("minio down").With("endpoint", endpoint).With("stack", stack.Trace().TrimRuntime())
	}

	// Create the test bucket and then place an empty index into it
	s3Client, errGo := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKeyOpt, *secretKeyOpt, ""),
		Secure: false,
	})
	if errGo != nil {
		return nil, endpoint, nil, kv.Wrap(errGo).With("endpoint", endpoint).With("stack", stack.Trace().TrimRuntime())
	}

	cleanUp, err = initTestBucket(s3Client, endpoint, bucket)
	return s3Client, endpoint, cleanUp, err
}

// waitForIndex will pause until the main server indexer runs a complete cycle then load a model for the tester
//
func waitForIndex(ctx context.Context, endpoint string, bucket string, key string) (mdl *model, err kv.Error) {
	for {
		// Wait for the server to complete an update pass
		WaitForScan(ctx)

		select {
		case <-ctx.Done():
			return nil, kv.NewError("model load stopped").With("endpoint", endpoint, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime())
		default:
		}

		// Now examine the server state for the presence of the index file with no blobs
		if mdl = GetModelIndex().Get(endpoint, key); mdl != nil {
			return mdl, nil
		}
	}
}

// TestModelUnload will use a single model and delete various blobs in various combinations from it
// and then will wait to see the results on the loaded model collection inside the server.  This checks
// progressive model changes that reduce the blob inventory in multiple ways.
func TestModelUnload(t *testing.T) {
	bucket := *bucketOpt
	objsCreated := []minio.ObjectInfo{}

	s3Client, endpoint, cleanUp, err := initTestWithMinio("", bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanUp(s3Client, bucket, objsCreated)
	}()

	// Used by the index file later
	payload := strings.Builder{}

	// Start with four blobs,  upload the index and check it
	blobHighWater := 4
	allBlobs := make([]minio.ObjectInfo, 0, blobHighWater)
	for aBlob := 0; aBlob != blobHighWater; aBlob++ {
		key := xid.New().String() + ".dat"
		data := runner.RandomString(rand.Intn(8192-4096) + 4096)
		uploadInfo, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(data)), int64(len(data)),
			minio.PutObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}
		// Get the ObjectInfo for the new blob and add it to the cleanup list
		objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		objsCreated = append(objsCreated, objInfo)
		allBlobs = append(allBlobs, objInfo)

		payload.WriteString(fmt.Sprintf("%s,%s\n", key, uploadInfo.ETag))
	}

	// Now create an index file with the four blobs
	key := indexPrefix + xid.New().String() + indexSuffix
	_, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
		minio.PutObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	// Get the ObjectInfo for the new blob and add it to the cleanup list
	objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	objsCreated = append(objsCreated, objInfo)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	mdl, err := waitForIndex(ctx, s3Client.EndpointURL().String(), bucket, key)
	if err != nil {
		t.Fatal(err)
	}

	if blobHighWater != len(mdl.blobs) {
		logger.Debug("Test results", "mdl", spew.Sdump(mdl), "stack", stack.Trace().TrimRuntime())
		t.Fatal(kv.NewError("model loaded incorrect number of items").With("endpoint", endpoint, "bucket", bucket, "key", key, "expected blobs", blobHighWater, "actual blobs", len(mdl.blobs)).With("stack", stack.Trace().TrimRuntime()))
	}

	// Now delete various blobs within the model and go back and check the desired result is what was wanted
	testCases := [][]minio.ObjectInfo{allBlobs[1:], allBlobs[:len(allBlobs)-2], allBlobs[1 : len(allBlobs)-3]}

	for _, testCase := range testCases {
		payload.Reset()
		for _, blob := range testCase {
			payload.WriteString(fmt.Sprintf("%s,%s\n", blob.Key, blob.ETag))
		}

		if _, errGo = s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
			minio.PutObjectOptions{}); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		mdl, err = waitForIndex(ctx, s3Client.EndpointURL().String(), bucket, key)
		if err != nil {
			t.Fatal(err)
		}

		if len(testCase) != len(mdl.blobs) {
			blobs := []string{}
			for k, _ := range mdl.blobs {
				blobs = append(blobs, k)
			}
			logger.Debug("Test results", "mdl", blobs, "stack", stack.Trace().TrimRuntime())
			blobs = []string{}
			for _, obj := range testCase {
				blobs = append(blobs, obj.Key)
			}
			logger.Debug("Test case", "mdl", blobs, "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model loaded incorrect number of items").With("endpoint", endpoint, "bucket", bucket, "key", key, "expected blobs", len(testCase), "actual blobs", len(mdl.blobs)).With("stack", stack.Trace().TrimRuntime()))
		}
		for _, obj := range testCase {
			if _, isPresent := mdl.blobs[obj.Key]; !isPresent {
				blobs := []string{}
				for k, _ := range mdl.blobs {
					blobs = append(blobs, k)
				}
				t.Fatal(kv.NewError("missing blob").With("endpoint", endpoint, "bucket", bucket, "key", obj.Key, "blobs", blobs).With("stack", stack.Trace().TrimRuntime()))
			}
		}
	}
}

// TestModelLoad will populate an S3 bucket with auto generated index file(s) of various sizes
// and check that they loads
//
func TestModelLoad(t *testing.T) {
	bucket := *bucketOpt
	objsCreated := []minio.ObjectInfo{}

	s3Client, endpoint, cleanUp, err := initTestWithMinio("", bucket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanUp(s3Client, bucket, objsCreated)
	}()

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
				t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
			}
			// Get the ObjectInfo for the new blob and add it to the cleanup list
			objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
			if errGo != nil {
				t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
			}

			objsCreated = append(objsCreated, objInfo)

			payload.WriteString(fmt.Sprintf("%s,%s\n", key, uploadInfo.ETag))
		}

		// Now create an empty index file
		key := indexPrefix + xid.New().String() + indexSuffix
		uploadInfo, errGo := s3Client.PutObject(context.Background(), bucket, key, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
			minio.PutObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		// Get the ObjectInfo for the new blob and add it to the cleanup list
		objInfo, errGo := s3Client.StatObject(context.Background(), bucket, key, minio.StatObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", endpoint, "bucket", bucket).With("stack", stack.Trace().TrimRuntime()))
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
			t.Fatal(kv.NewError("model loaded too many items").With("endpoint", endpoint, "bucket", bucket, "key", key, "expected blobs", i, "actual blobs", len(mdl.blobs)).With("stack", stack.Trace().TrimRuntime()))
		}
		if mdl.obj == nil {
			logger.Debug("Test results", "mdl", spew.Sdump(mdl), "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model info missing").With("endpoint", endpoint, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime()))
		}
		if strings.Trim(uploadInfo.ETag, "\"") != strings.Trim(mdl.obj.ETag, "\"") {
			logger.Debug("Test results", "mdlETag", mdl.obj.ETag, "uploaded ETag", uploadInfo.ETag, "stack", stack.Trace().TrimRuntime())
			t.Fatal(kv.NewError("model ETag unexpected").With("endpoint", endpoint, "bucket", bucket, "key", key).With("stack", stack.Trace().TrimRuntime()))
		}

		logger.Debug("Model index tested", "components", i, "stack", stack.Trace().TrimRuntime())
	}
}
