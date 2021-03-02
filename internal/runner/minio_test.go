// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"context"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
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
	size int64
}

type testData struct {
	name  string
	blobs []blob
}

func s3TestFiles(ctx context.Context, mts *minio_local.MinioTestServer, buckets int, blobs int) (bucketsAndBlobs []testData, err kv.Error) {

	// Create a list of generated IDs and sort then to ensure that blob handling
	// is predictable during testing
	ids := make([]string, 0, buckets*blobs+1+buckets)
	for i := 0; i != cap(ids); i++ {
		ids = append(ids, xid.New().String()+".bin")
	}
	sort.Strings(ids)

	// Create multiple buckets and upload to all
	id := ""
	bucketsAndBlobs = make([]testData, buckets, buckets)
	for i := 0; i != buckets; i++ {
		// pop front, https://ueokande.github.io/go-slice-tricks/
		id, ids = ids[0], ids[1:]
		bucketsAndBlobs[i] = testData{
			name:  id,
			blobs: make([]blob, blobs, blobs),
		}
		for j := 0; j != blobs; j++ {
			// pop front, https://ueokande.github.io/go-slice-tricks/
			id, ids = ids[0], ids[1:]
			bucketsAndBlobs[i].blobs[j] = blob{
				key:  id,
				size: int64(rand.Intn(8192-1024+1) + 1024),
			}
		}
	}

	// Cleanup after ourselves as best as we can on the remote minio server
	go func() {
		_ = <-ctx.Done()
		for _, bucket := range bucketsAndBlobs {
			mts.RemoveBucketAll(bucket.name)
		}
	}()

	for _, bucket := range bucketsAndBlobs {
		for _, blob := range bucket.blobs {
			if err := mts.UploadTestFile(bucket.name, blob.key, (int64)(blob.size)); err != nil {
				return bucketsAndBlobs, err
			}
		}
	}
	return bucketsAndBlobs, nil
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

	bucketsAndBlobs, err := s3TestFiles(ctx, mts, 2, 2)
	if err != nil {
		t.Fatal(err)
	}

	// Make the last bucket public and its contents
	if err := mts.SetPublic(bucketsAndBlobs[len(bucketsAndBlobs)-1].name); err != nil {
		t.Fatal(err)
	}

	// access using both secured and unsecured the buckets we have to validate access
	creds := request.AWSCredential{
		AccessKey: mts.AccessKeyId,
		SecretKey: mts.SecretAccessKeyId,
	}
	env := map[string]string{}

	for i, bucket := range bucketsAndBlobs {
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
		if i == len(bucketsAndBlobs)-1 {
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

type testCase struct {
	bucket string
	size   int64
	keys   []string
}

func limitTest(ctx context.Context, t *testing.T, mts *minio_local.MinioTestServer, creds request.AWSCredential, test testCase, logger *log.Logger) {

	// Create a new TMPDIR so that we can cleanup
	tmpDir, errGo := ioutil.TempDir("", "")
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer func() {
		os.RemoveAll(tmpDir)
	}()

	env := map[string]string{}

	authS3, err := s3.NewS3storage(ctx, creds, env, mts.Address, test.bucket, "", false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		authS3.Close()
	}()

	keys := make(map[string]int64, len(test.keys))

	// First we try downloading using the test case for positive case
	firstKey := ""
	maxBytes := int64(test.size)
	for _, key := range test.keys {
		size, warns, err := authS3.Fetch(ctx, key, false, tmpDir, maxBytes, nil)
		if err != nil {
			t.Fatal(err, "bucket", test.bucket, "key", key)
		}
		if len(warns) != 0 {
			t.Fatal(warns, "bucket", test.bucket, "key", key)
		}
		maxBytes -= size
		keys[key] = size
		if len(firstKey) == 0 {
			firstKey = key
		}
	}

	// Now examine the blobs that got downloaded and make sure they are all there
	files, errGo := os.ReadDir(tmpDir)
	if errGo != nil {
		t.Fatal(errGo)
	}

	for _, file := range files {
		if _, isPresent := keys[file.Name()]; !isPresent {
			t.Fatal("unexpected key was downloaded", file.Name(), "keys", spew.Sdump(keys))
		}
		if _, isPresent := keys[file.Name()]; !isPresent {
			t.Fatal("unexpected key was downloaded", file.Name(), "dir", tmpDir, "requested", test.size, "keys", spew.Sdump(keys))
		}
		delete(keys, file.Name())

		if errGo = os.Remove(filepath.Join(tmpDir, file.Name())); errGo != nil {
			t.Fatal(errGo)
		}
	}
	if len(keys) != 0 {
		t.Fatal("key(s) not downloaded", keys)
	}

	if files, errGo = os.ReadDir(tmpDir); errGo != nil {
		t.Fatal(errGo)
	}
	for _, file := range files {
		os.Remove(filepath.Join(tmpDir, file.Name()))
	}

	// Now try the gather to exercise our negative test of trying to download too much, the prefix
	// is not needed as we are using the entire bucket
	size, warnings, _ := authS3.Gather(ctx, "", tmpDir, test.size, nil, true)
	// err is ignored as we will go over budgets in testing to get our negative cases
	if size > test.size {
		t.Fatal("it appears Gather downloaded too much data", "actual", size, "expected", test.size)
	}

	if len(warnings) != 0 {
		t.Fatal(warnings)
	}

	// Release the keys from the test specification as the first pass
	// erases them intentionally
	for _, key := range test.keys {
		keys[key] = 0
	}

	if files, errGo = os.ReadDir(tmpDir); errGo != nil {
		t.Fatal(errGo)
	}

	for _, file := range files {
		if _, isPresent := keys[file.Name()]; !isPresent {
			t.Fatal("unexpected key was downloaded", file.Name(), "dir", tmpDir, "requested", test.size, "keys", spew.Sdump(keys))
		}
		delete(keys, file.Name())

		if errGo = os.Remove(filepath.Join(tmpDir, file.Name())); errGo != nil {
			t.Fatal(errGo)
		}
	}
	if len(keys) != 0 {
		t.Fatal("key(s) not downloaded", keys)
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
	bucketsAndBlobs, err := s3TestFiles(ctx, mts, 2, 8)
	if err != nil {
		t.Fatal(err)
	}

	// Use the Storage API to connect to the server
	creds := request.AWSCredential{
		AccessKey: mts.AccessKeyId,
		SecretKey: mts.SecretAccessKeyId,
	}

	tests := []testCase{}

	// Use the fetch calls to test limitations
	for _, bucket := range bucketsAndBlobs {
		// Generate a test case where we choose and blob to stop in at random
		// and a test case where we do not get to load any blobs, and one where
		// we load all blobs
		blobsTotal := int64(0)
		blobSizes := make([]int64, 0, len(bucket.blobs))
		keys := make([]string, 0, len(bucket.blobs))
		for _, blob := range bucket.blobs {
			blobSizes = append(blobSizes, blob.size)
			blobsTotal += blob.size
			keys = append(keys, blob.key)
		}
		// No blobs should be downloaded when the space is too small
		tests = append(tests, testCase{
			bucket: bucket.name,
			size:   blobSizes[0] / 2,
			keys:   []string{},
		})
		// All blobs should be downloaded when the space it exactly the amount needed
		tests = append(tests, testCase{
			bucket: bucket.name,
			size:   blobsTotal,
			keys:   append([]string{}, keys...),
		})
		// With space to spare all blobs should be downloaded
		tests = append(tests, testCase{
			bucket: bucket.name,
			size:   blobsTotal + 1,
			keys:   append([]string{}, keys...),
		})
		// Choose the middle blob as the limit and then use limits on either side of its accumulated
		// limits
		midPoint := len(keys)/2 + 1
		bisected := keys[:midPoint]
		biTot := int64(0)
		for _, blob := range bucket.blobs[:midPoint] {
			biTot += blob.size
		}
		// With 1/2 the blobs having sufficeint space lets see if the right number are downloaded
		tests = append(tests, testCase{
			bucket: bucket.name,
			size:   biTot,
			keys:   append([]string{}, bisected...),
		})
		// With 1/2 the blobs having sufficeint space with a small exccess lets see if the right number are downloaded
		tests = append(tests, testCase{
			bucket: bucket.name,
			size:   biTot + 1,
			keys:   append([]string{}, bisected...),
		})
	}
	for _, test := range tests {
		limitTest(ctx, t, mts, creds, test, logger)
	}
}

// TestS3Anon will test anonymous access to S3 public resources using Minio
//
func TestS3MinioAnon(t *testing.T) {

	logger := log.NewLogger("s3_anon_access")

	initMinio.Do(startMinio)

	s3AnonAccess(t, mts, logger)
}

// TestS3Limiter is used to test that the storage APIs inside the runner can successfully limit
// file system comsumption during downloads
//
func TestS3Limiter(t *testing.T) {
	logger := log.NewLogger("s3_limiter")

	initMinio.Do(startMinio)

	s3Limiter(t, mts, logger)
}
