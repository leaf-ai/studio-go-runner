// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains an number of explicit unit tests design to
// validate the caching layer that is difficult to do in a black box
// functional test.

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/pkg/studio"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/karlmutch/ccache"

	humanize "github.com/dustin/go-humanize"

	"github.com/rs/xid" // MIT
)

func okToTest(pth string) (err kv.Error) {

	minFree := uint64(100 * 1024 * 1024) // 100 Mbytes free is the minimum to do our cache tests

	free, err := runner.GetPathFree(pth)
	if err != nil {
		return err
	}

	if free < 100*1024*1024 {
		return kv.NewError("insufficient disk space").With("path", pth).With("needed", humanize.Bytes(minFree)).With("free", humanize.Bytes(free)).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

func TestCacheBase(t *testing.T) {
	cache := ccache.New(ccache.Configure().GetsPerPromote(1).MaxSize(5).ItemsToPrune(1))
	for i := 0; i < 7; i++ {
		cache.Set(strconv.Itoa(i), i, time.Minute)
	}
	time.Sleep(time.Millisecond * 10)
	if cache.Get("0") != nil {
		t.Fatal(kv.NewError("unexpected entry in cache").With("stack", stack.Trace().TrimRuntime()))
	}
	if cache.Get("1") != nil {
		t.Fatal(kv.NewError("unexpected entry in cache").With("stack", stack.Trace().TrimRuntime()))
	}
	if cache.Get("2").Value() != 2 {
		t.Fatal(kv.NewError("expected entry NOT in cache").With("stack", stack.Trace().TrimRuntime()))
	}
	logger.Info("TestCacheBase completed")
}

// TestCacheLoad will validate that fetching a file with an empty cache will
// trigger a fetch and then immediately followed by the same fetch will
// trigger a cache hit
//
func TestCacheLoad(t *testing.T) {

	pClient := NewPrometheusClient(fmt.Sprintf("http://localhost:%d/metrics", studio.GetPrometheusPort()))

	if !CacheActive {
		t.Skip("cache not activate")
	}

	// Check that we have sufficient resources, e.g. disk space, for the test
	if err := okToTest(os.TempDir()); err != nil {
		t.Fatal(err)
	}

	// This will erase any files from the artifact cache so that the test can
	// run unobstructed
	if err := runner.ClearObjStore(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.ClearObjStore() }()

	// Check that the minio local server has initialized before continuing
	ctx := context.Background()

	timeoutAlive, aliveCancel := context.WithTimeout(ctx, time.Minute)
	defer aliveCancel()

	if alive, err := runner.MinioTest.IsAlive(timeoutAlive); !alive || err != nil {
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("The minio test server is not available to run this test")
	}
	logger.Info("Alive checked", "addr", runner.MinioTest.Address)

	bucket := "testcacheload"
	fn := "file-1"

	if err := runner.MinioTest.UploadTestFile(bucket, fn, humanize.MiByte); err != nil {
		t.Fatal(err)
	}

	defer func() {
		for _, err := range runner.MinioTest.RemoveBucketAll(bucket) {
			logger.Warn(err.Error())
		}
	}()

	tmpDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// Build an artifact cache in the same manner as is used by the main studioml
	// runner implementation
	artifactCache = runner.NewArtifactCache()

	art := runner.Artifact{
		Bucket:    bucket,
		Key:       fn,
		Mutable:   false,
		Unpack:    false,
		Qualified: fmt.Sprintf("s3://%s/%s/%s", runner.MinioTest.Address, bucket, fn),
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     runner.MinioTest.AccessKeyId,
		"AWS_SECRET_ACCESS_KEY": runner.MinioTest.SecretAccessKeyId,
		"AWS_DEFAULT_REGION":    "us-west-2",
	}

	hash, err := artifactCache.Hash(ctx, &art, "project", tmpDir, "", env, "")
	if err != nil {
		t.Fatal(err)
	}

	// Extract the starting metrics for the server under going this test
	hits, misses, err := pClient.GetHitsMisses(hash)
	if err != nil {
		t.Fatal(err)
	}

	// In production the files would be downloaded to an experiment dir,
	// in the testing case we use a temporary directory as your artifact
	// group then wipe it when the test is done
	//
	warns, err := artifactCache.Fetch(ctx, &art, "project", tmpDir, "", env, "")
	if err != nil {
		for _, w := range warns {
			logger.Warn(w.Error())
		}
		t.Fatal(err)
	}

	// Run a fetch and ensure we have a miss and no change to the hits
	//
	newHits, newMisses, err := pClient.GetHitsMisses(hash)
	if err != nil {
		t.Fatal(err)
	}

	// Run a fetch and ensure we have a miss and no change to the hits
	if misses+1 != newMisses {
		t.Fatal(kv.NewError("new file did not result in a miss").With("hash", hash).With("stack", stack.Trace().TrimRuntime()))
	}
	if hits != newHits {
		t.Fatal(kv.NewError("new file unexpectedly resulted in a hit").With("hash", hash).With("stack", stack.Trace().TrimRuntime()))
	}

	// Refetch the file
	logger.Info("fetching file from warm cache")
	if warns, err = artifactCache.Fetch(ctx, &art, "project", tmpDir, "", env, ""); err != nil {
		for _, w := range warns {
			logger.Warn(w.Error())
		}
		t.Fatal(err)
	}

	newHits, newMisses, err = pClient.GetHitsMisses(hash)
	if err != nil {
		t.Fatal(err)
	}
	if hits+1 != newHits {
		t.Fatal(kv.NewError("existing file did not result in a hit when cache active").With("hash", hash).With("hits", newHits).With("misses", newMisses).With("stack", stack.Trace().TrimRuntime()))
	}
	if misses+1 != newMisses {
		t.Fatal(kv.NewError("existing file resulted in a miss when cache active").With("hash", hash).With("stack", stack.Trace().TrimRuntime()))
	}

	logger.Info("TestCacheLoad completed")
}

// TestCacheXhaust will fill the cache to capacity with 11 files each of 10% the size
// of the cache and will then make sure that the first file was groomed out by
// the subsequent loads
//
func TestCacheXhaust(t *testing.T) {

	if testing.Short() {
		t.Skip("skipping cache exhaustion in short mode")
	}

	prometheusURL := fmt.Sprintf("http://localhost:%d/metrics", studio.GetPrometheusPort())

	if !CacheActive {
		t.Skip("cache not activate")
	}

	// Check that we have sufficient resources, e.g. disk space, for the test
	if err := okToTest(os.TempDir()); err != nil {
		t.Fatal(err)
	}

	// This will erase any files from the artifact cache so that the test can
	// run unobstructed
	runner.ClearObjStore()
	defer runner.ClearObjStore()

	// Determine how the files should look in order to overflow the cache and loose the first
	// one
	bucket := "testcachexhaust"

	filesInCache := 10
	cacheMax := runner.ObjStoreFootPrint()
	fileSize := cacheMax / int64(filesInCache)

	// Create a single copy of a test file that will be uploaded multiple times
	tmpDir, fn, err := runner.TmpDirFile(fileSize)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tmpDir)

	// Recycle the same input file multiple times and upload, changing 1 byte
	// to get a different checksum in the cache for each one
	srcFn := fn
	for i := 1; i != filesInCache+2; i++ {

		key := fmt.Sprintf("%s-%02d", filepath.Base(fn), i)

		// Modify a single byte to force a change to the file hash
		f, errGo := os.OpenFile(srcFn, os.O_CREATE|os.O_WRONLY, 0644)
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("file", srcFn).With("stack", stack.Trace().TrimRuntime()))
		}
		if _, errGo = f.WriteAt([]byte{(byte)(i & 0xFF)}, 0); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("file", srcFn).With("stack", stack.Trace().TrimRuntime()))
		}
		if errGo = f.Close(); errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("file", srcFn).With("stack", stack.Trace().TrimRuntime()))
		}

		// Upload
		if err := runner.MinioTest.Upload(bucket, key, srcFn); err != nil {
			t.Fatal(err.Error())
		}
		logger.Info(key, stack.Trace().TrimRuntime())
	}

	// Build an artifact cache in the same manner as is used by the main studioml
	// runner implementation
	artifactCache = runner.NewArtifactCache()

	art := runner.Artifact{
		Bucket:  bucket,
		Mutable: false,
		Unpack:  false,
	}
	env := map[string]string{
		"AWS_ACCESS_KEY_ID":     runner.MinioTest.AccessKeyId,
		"AWS_SECRET_ACCESS_KEY": runner.MinioTest.SecretAccessKeyId,
		"AWS_DEFAULT_REGION":    "us-west-2",
	}

	ctx := context.Background()

	pClient := NewPrometheusClient(prometheusURL)

	// Now begin downloading checking the misses do occur, the highest numbers file being
	// the least recently used
	for i := filesInCache + 1; i != 0; i-- {
		key := fmt.Sprintf("%s-%02d", filepath.Base(fn), i)

		art.Key = key
		art.Qualified = fmt.Sprintf("s3://%s/%s/%s", runner.MinioTest.Address, bucket, key)

		hash, err := artifactCache.Hash(ctx, &art, "project", tmpDir, "", env, "")
		if err != nil {
			t.Fatal(err)
		}

		// Extract the starting metrics for the server under going this test
		hits, misses, err := pClient.GetHitsMisses(hash)
		if err != nil {
			t.Fatal(err)
		}
		logger.Info(key, hash, stack.Trace().TrimRuntime())

		// In production the files would be downloaded to an experiment dir,
		// in the testing case we use a temporary directory as your artifact
		// group then wipe it when the test is done
		//
		// Use a timeout during this test to catch any warnings that might have queued
		// up as failures occur internally and deal with these as failures if
		// the download does not complete during testing
		//
		fetchCtx, cancelFetchCtx := context.WithTimeout(ctx, time.Minute)
		warns, err := artifactCache.Fetch(fetchCtx, &art, "project", tmpDir, "", env, "")
		// If our local timeout occurred then we treat that as a failure for the test, as above
		if fetchCtx.Err() != nil {
			err = kv.Wrap(fetchCtx.Err()).With("stack", stack.Trace().TrimRuntime())
		}
		cancelFetchCtx()
		if err != nil {
			for _, w := range warns {
				logger.Warn(w.Error())
			}
			t.Fatal(err)
		}
		newHits, newMisses, err := pClient.GetHitsMisses(hash)
		if err != nil {
			t.Fatal(err)
		}
		if hits != newHits {
			t.Fatal(kv.NewError("new file resulted in a hit when cache active").With("hash", hash).
				With("hits", hits).With("misses", misses).
				With("newHits", newHits).With("newMisses", newMisses).
				With("stack", stack.Trace().TrimRuntime()))
		}
		if misses+1 > newMisses {
			t.Fatal(kv.NewError("new file did not result in a miss when cache active").With("hash", hash).
				With("hits", hits).With("misses", misses).
				With("newHits", newHits).With("newMisses", newMisses).
				With("stack", stack.Trace().TrimRuntime()))
		}
	}

	// Now go back in reverse order downloading making sure we get
	// hits until the pen-ultimate file.  This means we have exercised
	// all files except for the highest numbered of all files
	for i := 2; i != filesInCache+1; i++ {
		key := fmt.Sprintf("%s-%02d", filepath.Base(fn), i)

		art.Key = key
		art.Qualified = fmt.Sprintf("s3://%s/%s/%s", runner.MinioTest.Address, bucket, key)

		hash, err := artifactCache.Hash(ctx, &art, "project", tmpDir, "", env, "")
		if err != nil {
			t.Fatal(err)
		}

		// Extract the starting metrics for the server under going this test
		hits, misses, err := pClient.GetHitsMisses(hash)
		if err != nil {
			t.Fatal(err)
		}

		logger.Info(key, hash, stack.Trace().TrimRuntime())

		// In production the files would be downloaded to an experiment dir,
		// in the testing case we use a temporary directory as your artifact
		// group then wipe it when the test is done
		//
		warns, err := artifactCache.Fetch(ctx, &art, "project", tmpDir, "", env, "")
		if err != nil {
			for _, w := range warns {
				logger.Warn(w.Error())
			}
			t.Fatal(err)
		}
		newHits, newMisses, err := pClient.GetHitsMisses(hash)
		if err != nil {
			t.Fatal(err)
		}
		if hits+1 != newHits {
			t.Fatal(kv.NewError("existing file did not result in a hit when cache active").With("hash", hash).
				With("hits", hits).With("misses", misses).
				With("newHits", newHits).With("newMisses", newMisses).
				With("stack", stack.Trace().TrimRuntime()))
		}
		if misses != newMisses {
			t.Fatal(kv.NewError("existing file resulted in a miss when cache active").With("hash", hash).
				With("hits", hits).With("misses", misses).
				With("newHits", newHits).With("newMisses", newMisses).
				With("stack", stack.Trace().TrimRuntime()))
		}
	}

	logger.Trace("allowing the gc to kick in for the caching", stack.Trace().TrimRuntime())
	select {
	case TriggerCacheC <- struct{}{}:
		time.Sleep(3 * time.Second)
	case <-time.After(40 * time.Second):
	}
	logger.Debug("cache gc signalled", stack.Trace().TrimRuntime())

	// Check for a miss on the very last file that has been ignored for the longest
	i := filesInCache + 1
	key := fmt.Sprintf("%s-%02d", filepath.Base(fn), i)

	art.Key = key
	art.Qualified = fmt.Sprintf("s3://%s/%s/%s", runner.MinioTest.Address, bucket, key)

	hash, err := artifactCache.Hash(ctx, &art, "project", tmpDir, "", env, "")
	if err != nil {
		t.Fatal(err)
	}

	if runner.CacheProbe(hash) {
		t.Fatal(kv.NewError("cache still contained old key").With("hash", hash).
			With("stack", stack.Trace().TrimRuntime()))
	}

	logger.Info(key, hash, stack.Trace().TrimRuntime())

	// Extract the starting metrics for the server under going this test
	hits, misses, err := pClient.GetHitsMisses(hash)
	if err != nil {
		t.Fatal(err)
	}

	// In production the files would be downloaded to an experiment dir,
	// in the testing case we use a temporary directory as your artifact
	// group then wipe it when the test is done
	//
	warns, err := artifactCache.Fetch(ctx, &art, "project", tmpDir, "", env, "")
	if err != nil {
		for _, w := range warns {
			logger.Warn(w.Error())
		}
		t.Fatal(err)
	}
	newHits, newMisses, err := pClient.GetHitsMisses(hash)
	if err != nil {
		t.Fatal(err)
	}
	if hits != newHits {
		t.Fatal(kv.NewError("flushed file resulted in a hit when cache active").With("hash", hash).
			With("hits", hits).With("misses", misses).
			With("newHits", newHits).With("newMisses", newMisses).
			With("stack", stack.Trace().TrimRuntime()))
	}
	if misses+1 != newMisses {
		t.Fatal(kv.NewError("flushed file did not result in a miss when cache active").With("hash", hash).
			With("hits", hits).With("misses", misses).
			With("newHits", newHits).With("newMisses", newMisses).
			With("stack", stack.Trace().TrimRuntime()))
	}

	logger.Info("TestCacheXhaust completed")
}
