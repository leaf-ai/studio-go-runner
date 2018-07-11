package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/SentientTechnologies/studio-go-runner"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	humanize "github.com/dustin/go-humanize"

	"github.com/rs/xid" // MIT

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/prom2json" // Apache 2.0
)

func getMetrics(addr string) (result []*prom2json.Family, err errors.Error) {
	result = []*prom2json.Family{}

	mfC := make(chan *dto.MetricFamily, 1024)

	go func() {
		if errGo := prom2json.FetchMetricFamilies(addr, mfC, "", "", true); errGo != nil {
			err = errors.Wrap(errGo)
		}
	}()

	for mf := range mfC {
		result = append(result, prom2json.NewFamily(mf))
	}
	return result, err
}

func getHitsMisses(metricsURL string, fn string) (hits int, misses int, err errors.Error) {
	results, err := getMetrics(metricsURL)
	if err != nil {
		return 0, 0, err
	}

	hits = 0
	misses = 0
	for _, metric := range results {
		m, ok := metric.Metrics[0].(prom2json.Metric)
		if !ok {
			continue
		}
		if v, _ := m.Labels["file"]; v == fn {
			if metric.Name == "runner_cache_hits" {
				hits, _ = strconv.Atoi(metric.Metrics[0].(prom2json.Metric).Value)
			}
			if metric.Name == "runner_cache_misses" {
				misses, _ = strconv.Atoi(metric.Metrics[0].(prom2json.Metric).Value)
			}
		}
	}
	return hits, misses, nil
}

func uploadTestFile(bucket string, key string, size uint64) (err errors.Error) {
	cfgDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer os.RemoveAll(cfgDir)

	fn := path.Join(cfgDir, xid.New().String())
	f, errGo := os.Create(fn)
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if errGo = f.Truncate(1e7); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	defer os.Remove(fn)

	// Get the Minio Test Server instance and sent it some random data while generating
	// a hash
	return runner.MinioTest.Upload(bucket, key, fn)
}

// This file contains an number of explicit unit tests design to
// validate the caching layer that is difficult to do in a black box
// functional test.

func TestCacheLoad(t *testing.T) {

	if !CacheActive {
		t.Skip("cache not activate")
	}

	// This will erase any files from the artifact cache so that the test can
	// run unobstructed
	runner.ClearObjStore()

	logger = runner.NewLogger("cache_load_test")

	bucket := "testcacheload"
	fn := "file-1"

	if err := uploadTestFile(bucket, fn, humanize.MiByte); err != nil {
		t.Fatal(err)
	}

	tmpDir, errGo := ioutil.TempDir("", xid.New().String())
	if errGo != nil {
		t.Fatal(errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	defer os.RemoveAll(tmpDir)

	// Extract the starting metrics for the server under going this test
	hits, misses, err := getHitsMisses(fmt.Sprintf("http://localhost:%d/metrics", PrometheusPort), fn)
	if err != nil {
		t.Fatal(err)
	}

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
	// In production the files would be downloaded to an experiment dir,
	// in the testing case we use a temporary directory as your artifact
	// group then wipe it when the test is done
	//
	if err = artifactCache.Fetch(&art, "project", tmpDir, "", env, ""); err != nil {
		t.Fatal(err)
	}

	// Run a fetch and ensure we have a miss and no change to the hits
	//
	newHits, newMisses, err := getHitsMisses(fmt.Sprintf("http://localhost:%d/metrics", PrometheusPort), fn)
	if err != nil {
		t.Fatal(err)
	}

	// Run a fetch and ensure we have a miss and no change to the hits
	if misses+1 != newMisses {
		t.Fatal(errors.New("new file did not result in a miss").With("stack", stack.Trace().TrimRuntime()))
	}
	if hits != newHits {
		t.Fatal(errors.New("new file unexpectedly resulted in a hit").With("stack", stack.Trace().TrimRuntime()))
	}

	// Refetch the file
	logger.Info("fetching file from warm cache")
	if err = artifactCache.Fetch(&art, "project", tmpDir, "", env, ""); err != nil {
		t.Fatal(err)
	}

	newHits, newMisses, err = getHitsMisses(fmt.Sprintf("http://localhost:%d/metrics", PrometheusPort), fn)
	if err != nil {
		t.Fatal(err)
	}
	if hits+1 != newHits {
		t.Fatal(errors.New("existing file did not result in a hit when cache active").With("hits", newHits).With("misses", newMisses).With("stack", stack.Trace().TrimRuntime()))
	}
	if misses+1 != newMisses {
		t.Fatal(errors.New("existing file resulted in a miss when cache active").With("stack", stack.Trace().TrimRuntime()))
	}

	logger.Info("TestCacheLoad completed")
}
