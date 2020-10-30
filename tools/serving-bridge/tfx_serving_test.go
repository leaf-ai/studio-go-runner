package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/minio/minio-go/v7"
	"github.com/rs/xid"
)

// gatherFiles collects together the file names inside a directory
// recursively and saves them using the dir prefix supplied
func gatherFiles(dir string) (files []string, err kv.Error) {

	files = []string{}

	errGo := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			files = append(files, path)
			return nil
		})

	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return files, nil
}

// This file contains tests related to the TFX configuration file loader
// and configuration generator
//
func TestTFXCfgGenerator(t *testing.T) {
	objsCreated := []minio.ObjectInfo{}

	s3Client, cfg, cleanUp, err := initTestWithMinio()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanUp(s3Client, cfg.bucket, objsCreated)
	}()

	// Deploy a model by examining the example model directory and
	// generating an index file, then uploading all of the resulting
	// files to S3
	modelDir := filepath.Join(".", "model_gen", "model")
	baseDir := filepath.Join("model_gen", "model")

	files, err := gatherFiles(modelDir)
	if err != nil {
		t.Fatal(err)
	}

	// This will contain the index when we are done
	payload := strings.Builder{}

	for _, fn := range files {
		data, errGo := ioutil.ReadFile(fn)
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime()))
		}

		uploadInfo, errGo := s3Client.PutObject(context.Background(), cfg.bucket, fn, bytes.NewReader([]byte(data)), int64(len(data)),
			minio.PutObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
		}
		// Get the ObjectInfo for the new blob and add it to the cleanup list
		objInfo, errGo := s3Client.StatObject(context.Background(), cfg.bucket, fn, minio.StatObjectOptions{})
		if errGo != nil {
			t.Fatal(kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
		}

		objsCreated = append(objsCreated, objInfo)

		payload.WriteString(fmt.Sprintf("%s,%s,%s\n", baseDir, fn, uploadInfo.ETag))
	}

	indexKey := indexPrefix + xid.New().String() + indexSuffix
	_, errGo := s3Client.PutObject(context.Background(), cfg.bucket, indexKey, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
		minio.PutObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	// Get the ObjectInfo for the new blob and add it to the cleanup list
	objInfo, errGo := s3Client.StatObject(context.Background(), cfg.bucket, indexKey, minio.StatObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	objsCreated = append(objsCreated, objInfo)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	_, err = waitForIndex(ctx, s3Client.EndpointURL().String(), cfg.bucket, indexKey)
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug("debug", "stack", stack.Trace().TrimRuntime())
	TFXScanWait(ctx)
	logger.Debug("debug", "stack", stack.Trace().TrimRuntime())

	// Check that the TFX server retrieved it
	// Check that the TFX server generated a valid configuration file for the served mode
}
