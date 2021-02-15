package main

// This file contains tests related to the TFX configuration file loader
// and configuration generator

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

	"github.com/cenkalti/backoff/v4"
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

func TFXUploadModel(ctx context.Context, s3Client *minio.Client, cfg Config, modelDir string, baseDir string) (indexS3Info minio.ObjectInfo, objsCreated []minio.ObjectInfo, err kv.Error) {
	files, err := gatherFiles(modelDir)
	if err != nil {
		return indexS3Info, objsCreated, err
	}

	// This will contain the index when we are done
	payload := strings.Builder{}

	for _, fn := range files {
		data, errGo := ioutil.ReadFile(fn)
		if errGo != nil {
			return indexS3Info, objsCreated, kv.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
		}

		uploadInfo, errGo := s3Client.PutObject(context.Background(), cfg.bucket, fn, bytes.NewReader([]byte(data)), int64(len(data)),
			minio.PutObjectOptions{})
		if errGo != nil {
			return indexS3Info, objsCreated, kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime())
		}
		// Get the ObjectInfo for the new blob and add it to the cleanup list
		objInfo, errGo := s3Client.StatObject(context.Background(), cfg.bucket, fn, minio.StatObjectOptions{})
		if errGo != nil {
			return indexS3Info, objsCreated, kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime())
		}

		objsCreated = append(objsCreated, objInfo)

		payload.WriteString(fmt.Sprintf("%s,%s,%s\n", baseDir, fn, uploadInfo.ETag))
	}

	indexKey := indexPrefix + xid.New().String() + indexSuffix
	_, errGo := s3Client.PutObject(context.Background(), cfg.bucket, indexKey, bytes.NewReader([]byte(payload.String())), int64(len(payload.String())),
		minio.PutObjectOptions{})
	if errGo != nil {
		return indexS3Info, objsCreated, kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime())
	}

	// Get the ObjectInfo for the new blob and add it to the cleanup list
	indexS3Info, errGo = s3Client.StatObject(context.Background(), cfg.bucket, indexKey, minio.StatObjectOptions{})
	if errGo != nil {
		return indexS3Info, objsCreated, kv.Wrap(errGo).With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime())
	}

	_, err = waitForIndex(ctx, cfg.endpoint, cfg.bucket, indexKey)
	if err != nil {
		return indexS3Info, objsCreated, err
	}

	return indexS3Info, objsCreated, nil
}

// TestTFXKubernetesServing is used to perform a full cycle of model serving and prediction
// when the Kubernetes serving is loaded
func TestTFXServing(t *testing.T) {
}

// TestTFXCfgGenerator is used to copy model files to S3 and then to have a config file generated that mirrors what the
// TFX Serving image would make use of.
///
func TestTFXCfgGenerator(t *testing.T) {
	// Setup the retries policies for communicating with the S3 service endpoint
	backoffs := backoff.NewExponentialBackOff()
	backoffs.InitialInterval = time.Duration(10 * time.Second)
	backoffs.Multiplier = 1.5
	backoffs.MaxElapsedTime = backoffs.InitialInterval * 5
	backoffs.Stop = backoffs.InitialInterval * 4

	objsCreated := []minio.ObjectInfo{}

	s3Client, cfg, cleanUp, err := initTestWithMinio()
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		cleanUp(s3Client, cfg.bucket, objsCreated)

		if r := recover(); r != nil {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		// Wait for the index reader to do a complete update pass before continuing
		IndexScanWait(ctx)

		// Wait for the TFX server to signal that it has updated its state
		TFXScanWait(ctx)

		count, _, err := bucketStats(context.Background(), cfg, backoffs)
		if err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal(kv.NewError("bucket not empty, needed post condition").With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	logger.Debug("stack", stack.Trace().TrimRuntime())
	// Wait for the index reader to do a complete update pass before continuing
	IndexScanWait(ctx)

	logger.Debug("stack", stack.Trace().TrimRuntime())
	// Wait for the TFX server to signal that it has updated its state
	TFXScanWait(ctx)

	logger.Debug("stack", stack.Trace().TrimRuntime())
	count, _, err := bucketStats(ctx, cfg, backoffs)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal(kv.NewError("bucket not empty, pre-requisite").With("endpoint", cfg.endpoint, "bucket", cfg.bucket).With("stack", stack.Trace().TrimRuntime()))
	}

	// Check that the TFX server generated an empty vconfiguration file
	tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
	if err != nil {
		t.Fatal(err)
	}

	// Check we have a single model and the base directory is valid
	cfgListVariant, ok := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList)
	if ok && cfgListVariant != nil {
		cfgList := cfgListVariant.ModelConfigList.Config
		if len(cfgList) != 0 {
			err = kv.NewError("model mix was not empty, a test prerequisite").With("endpoint", cfg.endpoint, "bucket", cfg.bucket, "cfg_list", SpewSmall.Sdump(cfgList)).With("stack", stack.Trace().TrimRuntime())
			logger.Error(err.Error())
			t.Fatal(err)
		}
	}
	// Deploy a model by examining the example model directory and
	// generating an index file, then uploading all of the resulting
	// files to S3
	modelDir := filepath.Join(".", "model_gen", "model")
	baseDir := filepath.Join("model_gen", "model")

	indexS3Info, created, err := TFXUploadModel(ctx, s3Client, cfg, modelDir, baseDir)
	if err != nil {
		t.Fatal(err)
	}
	objsCreated = append(objsCreated, created...)

	logger.Debug("stack", stack.Trace().TrimRuntime())
	// Wait for the TFX server to signal that it has updated its state
	TFXScanWait(ctx)

	// Check that the TFX server generated a valid configuration file for the served model
	if tfxCfg, err = ReadTFXCfg(ctx, cfg, logger); err != nil {
		t.Fatal(err)
	}

	// Check we have a single model and the base directory is valid
	cfgList := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config
	if len(cfgList) != 1 {
		t.Fatal(kv.NewError("model mix was not correct").With("endpoint", cfg.endpoint, "bucket", cfg.bucket, "cfg_list", SpewSmall.Sdump(cfgList)).With("stack", stack.Trace().TrimRuntime()))
	}
	if diff := deep.Equal(cfgList[0].BasePath, "s3://"+cfg.bucket+"/model_gen/model/"); diff != nil {
		t.Fatal(diff)
	}

	// Now remove the index and check to see if the model goes away
	cleanUp(s3Client, cfg.bucket, []minio.ObjectInfo{indexS3Info})

	func() {
		for {
			logger.Debug("stack", stack.Trace().TrimRuntime())
			// Wait for the TFX server to signal that is has updated its state
			TFXScanWait(ctx)

			select {
			case <-ctx.Done():
				logger.Debug("timeout wait for empty configuration", "stack", stack.Trace().TrimRuntime())
				return
			default:
			}

			logger.Debug("stack", stack.Trace().TrimRuntime())
			// Check that the TFX server generated a valid configuration file for the served mode
			tfxCfg, err := ReadTFXCfg(ctx, cfg, logger)
			if err != nil {
				logger.Debug(Spew.Sdump(cfg), "stack", stack.Trace().TrimRuntime())
				t.Fatal(err)
			}

			// Check we have a single model and the base directory is valid
			cfgList := tfxCfg.Config.(*serving_config.ModelServerConfig_ModelConfigList).ModelConfigList.Config
			if len(cfgList) == 0 {
				return
			}

		}
	}()
	logger.Debug("stack", stack.Trace().TrimRuntime())
}
