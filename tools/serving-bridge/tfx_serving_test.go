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
	"github.com/go-test/deep"
	"github.com/jjeffery/kv"
	serving_config "github.com/leaf-ai/studio-go-runner/internal/gen/tensorflow_serving/config"
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

	logger.Debug(Spew.Sdump(cfg), "stack", stack.Trace().TrimRuntime())

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
	indexS3Info, errGo := s3Client.StatObject(context.Background(), cfg.bucket, indexKey, minio.StatObjectOptions{})
	if errGo != nil {
		t.Fatal(kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	_, err = waitForIndex(ctx, s3Client.EndpointURL().String(), cfg.bucket, indexKey)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the TFX server to signal that is has updated its state
	TFXScanWait(ctx)

	// Get the server configuration

	logger.Debug("debug", "filename", cfg.tfxConfigFn, "stack", stack.Trace().TrimRuntime())

	// Check that the TFX server generated a valid configuration file for the served mode
	tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	logger.Debug(Spew.Sdump(tfxCfg), "stack", stack.Trace().TrimRuntime())

	// Check we have a single model and the base directory is valid
	cfgList := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config
	if len(cfgList) != 1 {
		t.Fatal(kv.NewError("model was not configured for serving").With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
	}
	if diff := deep.Equal(cfgList[0].BasePath, "model_gen/model"); diff != nil {
		t.Fatal(diff)
	}

	// Now remove the index and check to see if the model goes away
	cleanUp(s3Client, cfg.bucket, []minio.ObjectInfo{indexS3Info})

	func() {
		for {
			// Wait for the TFX server to signal that is has updated its state
			TFXScanWait(ctx)

			select {
			case <-ctx.Done():
				logger.Debug("timeout wait for empty configuration", "stack", stack.Trace().TrimRuntime())
				return
			default:
			}

			// Check that the TFX server generated a valid configuration file for the served mode
			tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
			if err != nil {
				logger.Debug(Spew.Sdump(cfg), "stack", stack.Trace().TrimRuntime())
				t.Fatal(err)
			}

			logger.Debug(Spew.Sdump(tfxCfg), "stack", stack.Trace().TrimRuntime())

			// Check we have a single model and the base directory is valid
			cfgList := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config
			if len(cfgList) == 0 {
				return
			}

		}
	}()
}
