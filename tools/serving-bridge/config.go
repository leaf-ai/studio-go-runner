// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file implements a configuration block for the server.  This configuration block represents a
// complete block that is passed to modules listening for configuration changes.  An
// additional ConfigOptionals block is defined in the broadcast_cfg.go file that represents a selected
// number of items that are to be applied to the full configuration block in this file.

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"github.com/go-stack/stack"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/fsnotify/fsnotify"

	"github.com/jjeffery/kv"
)

var (
	endpointOpt  = flag.String("aws-endpoint", "", "In the case of minio this should be a hostname, for aws please use \"s3.amazonaws.com\"")
	accessKeyOpt = flag.String("aws-access-key-id", "", "mandatory credentials for accessing S3 storage")
	secretKeyOpt = flag.String("aws-secret-access-key", "", "mandatory credentials for accessing S3 storage")
	bucketOpt    = flag.String("aws-bucket", "model-serving", "The name of the bucket which will be scanned for CSV index files")

	tfxConfigOpt   = flag.String("tfx-config-name", "", "The name for the TFX serving facility configuration file")
	tfxConfigCMOpt = flag.String("tfx-config-map-name", "", "The Kubernetes config map name for the TFX serving facility configuration file")
)

type Config struct {
	endpoint  string // S3 Host endpoint
	secretKey string // S3 secretKey
	accessKey string // S3 accessKey
	bucket    string // S3 Bucket

	tfxConfigFn string // TFX Serving Configuration file, https://www.tensorflow.org/tfx/serving/serving_config#model_server_config_details
	tfxConfigCM string // TFX Serving Configuration Kubernets Config map, https://www.tensorflow.org/tfx/serving/serving_config#model_server_config_details
}

func GetDefaultCfg() (cfg *Config, err kv.Error) {
	cfg = &Config{
		endpoint:    *endpointOpt,
		accessKey:   *accessKeyOpt,
		secretKey:   *secretKeyOpt,
		bucket:      *bucketOpt,
		tfxConfigFn: *tfxConfigOpt,
		tfxConfigCM: *tfxConfigOpt,
	}
	return cfg, nil
}

// WaitForMinioTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func WaitForMinioTest(ctx context.Context, cfgUpdater *Listeners) (alive bool, err kv.Error) {

	if alive, err := runner.MinioTest.IsAlive(ctx); !alive || err != nil {
		return false, err
	}

	if cfgUpdater != nil {

		bucket := (*bucketOpt)[:]
		tfxConfigFn := (*tfxConfigOpt)[:]
		tfxConfigCM := (*tfxConfigCMOpt)[:]

		cfg := ConfigOptionals{
			endpoint:  &runner.MinioTest.Address,
			accessKey: &runner.MinioTest.AccessKeyId,
			secretKey: &runner.MinioTest.SecretAccessKeyId,

			bucket:      &bucket,
			tfxConfigFn: &tfxConfigFn,
			tfxConfigCM: &tfxConfigCM,
		}
		select {
		case cfgUpdater.SendingC <- cfg:
		case <-ctx.Done():
		}

		if logger.IsTrace() {
			logger.Trace("server minio details", "cmd line", *endpointOpt, "effective", Spew.Sdump(cfg))
		} else {
			logger.Debug("server minio details", "cmd line", *endpointOpt, "effective", strings.ReplaceAll(SpewSmall.Sdump(cfg), "\n", ""))
		}
	}
	return true, nil
}

// startCfgUpdater is use to initiate a watcher on the mounted directory for new configuration
// values using the norms set down by Kubernetes ConfigMap based mounts
//
func startCfgUpdater(ctx context.Context, cfgUpdater *Listeners, mount string, errorC chan kv.Error) {
	if len(mount) == 0 {
		logger.Warn("mount for dynamic configuration not activated")
		return
	}

	watcher, errGo := fsnotify.NewWatcher()
	if errGo != nil {
		errorC <- kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		return
	}

	go cfgUpdaterRun(ctx, watcher, cfgUpdater, mount, errorC)
}

func cfgSendError(err kv.Error, errorC chan kv.Error, lastError kv.Error, repeatTime time.Time) (rLastError kv.Error, rRepeatTime time.Time) {
	if err.Error() == lastError.Error() {
		if repeatTime.After(time.Now()) {
			return err, repeatTime
		}
	}
	errorC <- err
	return err, time.Now().Add(10 * time.Minute)
}

func cfgUpdaterAdd(ctx context.Context, watcher *fsnotify.Watcher, cfgUpdater *Listeners, mount string, errorC chan kv.Error) {

	refresh := time.NewTicker(10 * time.Second)
	defer refresh.Stop()

	// The following are used to suppress multiple duplicate errors
	// occurring within short periods of trime
	lastError := kv.NewError("")
	repeatError := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh.C:
			info, errGo := os.Stat(mount)
			if errGo != nil {
				err := kv.Wrap(errGo).With("mount", mount).With("stack", stack.Trace().TrimRuntime())
				lastError, repeatError = cfgSendError(err, errorC, lastError, repeatError)
				continue
			}
			if !info.IsDir() {
				err := kv.NewError("mount exists but is not a directory").With("mount", mount).With("stack", stack.Trace().TrimRuntime())
				lastError, repeatError = cfgSendError(err, errorC, lastError, repeatError)
				continue
			}
			if errGo = watcher.Add(mount); errGo == nil {
				logger.Trace("Debug", "Added", mount, "stack", stack.Trace().TrimRuntime())
				return
			}
			err := kv.Wrap(errGo).With("mount", mount).With("stack", stack.Trace().TrimRuntime())
			lastError, repeatError = cfgSendError(err, errorC, lastError, repeatError)
		}
	}
}

func cfgUpdaterRun(ctx context.Context, watcher *fsnotify.Watcher, cfgUpdater *Listeners, mount string, errorC chan kv.Error) {
	defer watcher.Close()

	// Asynchronous run the function to add the watcher for the user specified directory
	// until it is successful or the server terminates
	go cfgUpdaterAdd(ctx, watcher, cfgUpdater, mount, errorC)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			logger.Warn("event:", event)
			if event.Op&fsnotify.Write == fsnotify.Write {
				logger.Warn("modified file:", event.Name)
			}
		case errGo, ok := <-watcher.Errors:
			if !ok {
				return
			}
			errorC <- kv.Wrap(errGo).With("mount", mount).With("stack", stack.Trace().TrimRuntime())
		case <-ctx.Done():
			return
		}
	}
}
