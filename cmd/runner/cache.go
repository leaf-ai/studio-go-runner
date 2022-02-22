// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// The file contains the implementation of functions related to starting and maintaining a
// disk cache for the artifacts being used by the runner

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	objCacheOpt    = flag.String("cache-dir", "", "An optional directory to be used as a cache for downloaded artifacts")
	objCacheMaxOpt = flag.String("cache-size", "", "The maximum target size of the disk based download cache, for example (10Gb), must be larger than 1Gb")

	// CacheActive is set to true if or when the caching system has been configured and is activated
	CacheActive = false
)

func getCacheOptions() (dir string, size int64, err kv.Error) {
	dir = *objCacheOpt

	if len(*objCacheOpt) != 0 || len(*objCacheMaxOpt) != 0 {
		if len(*objCacheOpt) == 0 {
			return dir, size, kv.Wrap(fmt.Errorf("if the option cache-size is specified the cache-dir must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
		if len(*objCacheMaxOpt) == 0 {
			return dir, size, kv.Wrap(fmt.Errorf("if the option cache-dir is specified the cache-size must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
	}

	if len(*objCacheMaxOpt) != 0 {
		size, errGo := humanize.ParseBytes(*objCacheMaxOpt)
		if errGo != nil {
			return "", int64(size), kv.Wrap(errGo, "option cache-size was not formatted correctly").With("stack", stack.Trace().TrimRuntime())
		}

		if size < 1024*1024*1024 {
			return "", int64(size), kv.NewError("option cache-size was ignored, too small to be useful, less than 1Gb").With("stack", stack.Trace().TrimRuntime())
		}
		return dir, int64(size), nil
	}
	return "", 0, nil
}

func startObjStore(ctx context.Context, removedC chan os.FileInfo, errorC chan kv.Error) (enabled bool, err kv.Error) {

	dir, size, err := getCacheOptions()
	if err != nil {
		return false, err
	}

	if size == 0 || len(dir) == 0 {
		logger.Warn("cache not being used")
		return false, nil
	}

	// Create the cache directory if it doesn't exist yet
	_ = os.MkdirAll(dir, 0700)

	err = runner.InitObjStore(ctx, dir, size, removedC, errorC)

	return true, err
}

func runObjCache(ctx context.Context) (err kv.Error) {

	removedC := make(chan os.FileInfo, 1)
	errorC := make(chan kv.Error, 3)

	go func() {
		defer func() {
			logger.Warn("cache service stopped")
			defer func() {
				_ = recover()
			}()

			close(errorC)
			close(removedC)
			_ = os.RemoveAll(*objCacheOpt)
		}()
		for {
			select {
			case err := <-errorC:
				if err != nil {
					logger.Info(err.Error())
				}
			case removed := <-removedC:
				if removed == nil {
					return
				}
				logger.Info(fmt.Sprintf("removed %#v from cache", removed.Name()))
			case <-ctx.Done():
				return
			}
		}
	}()

	CacheActive, err = startObjStore(ctx, removedC, errorC)
	if err != nil {
		return err
	}
	if !CacheActive {

		defer func() {
			defer func() {
				_ = recover()
			}()
			close(removedC)
		}()

		return nil
	}
	return nil
}
