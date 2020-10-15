// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"os"
	"time"

	"github.com/go-stack/stack"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/fsnotify/fsnotify"

	"github.com/jjeffery/kv"
)

// This file contains the implementation of a configuration block for this
// server

type Config struct {
	endpoint  string
	secretKey string
	accessKey string
	bucket    string
}

// WaitForMinioTest is intended to block until such time as a testing minio server is
// found.  It will also update the server CLI config items to reflect the servers presence.
//
func WaitForMinioTest(ctx context.Context, cfgUpdater *Listeners) (alive bool, err kv.Error) {

	if alive, err := runner.MinioTest.IsAlive(ctx); !alive || err != nil {
		return false, err
	}

	logger.Trace("server minio details", "cmd line", *endpointOpt, "effective", runner.MinioTest.Address)

	if cfgUpdater != nil {
		cfg := ConfigOptionals{
			endpoint:  &runner.MinioTest.Address,
			accessKey: &runner.MinioTest.AccessKeyId,
			secretKey: &runner.MinioTest.SecretAccessKeyId,
		}
		cfgUpdater.SendingC <- cfg
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
