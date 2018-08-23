package main

// The file contains the implementation of functions related to starting and maintaining a
// disk cache for the artifacts being used by the runner

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	objCacheCreate = flag.Bool("cache-create", false, "Create the cache directory when starting, remove it when the process exits, this may leak in the event of an unrecoverable exception, use of this is profane except during testing")
	objCacheOpt    = flag.String("cache-dir", "", "An optional directory to be used as a cache for downloaded artifacts")
	objCacheMaxOpt = flag.String("cache-size", "", "The maximum target size of the disk based download cache, for example (10Gb), must be larger than 1Gb")

	// Set to true if or when the caching system has been configured and is activated
	CacheActive = false
)

func getCacheOptions() (dir string, size int64, err errors.Error) {
	dir = *objCacheOpt

	if len(*objCacheOpt) != 0 || len(*objCacheMaxOpt) != 0 {
		if len(*objCacheOpt) == 0 {
			return dir, size, errors.Wrap(fmt.Errorf("if the option cache-size is specified the cache-dir must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
		if len(*objCacheMaxOpt) == 0 {
			return dir, size, errors.Wrap(fmt.Errorf("if the option cache-dir is specified the cache-size must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
	}

	if len(*objCacheMaxOpt) != 0 {
		size, errGo := humanize.ParseBytes(*objCacheMaxOpt)
		if errGo != nil {
			return "", int64(size), errors.Wrap(errGo, "option cache-size was not formatted correctly").With("stack", stack.Trace().TrimRuntime())
		}

		if size < 1024*1024*1024 {
			return "", int64(size), errors.New("option cache-size was ignored, too small to be useful, less than 1Gb").With("stack", stack.Trace().TrimRuntime())
		}
		return dir, int64(size), nil
	}
	return "", 0, nil
}

func startObjStore(ctx context.Context, removedC chan os.FileInfo, errorC chan errors.Error) (enabled bool, err errors.Error) {

	dir, size, err := getCacheOptions()
	if err != nil {
		return false, err
	}

	if size == 0 || len(dir) == 0 {
		logger.Warn("cache not being used")
		return false, nil
	}

	// Create the cache directory if asked too
	if *objCacheCreate {
		os.MkdirAll(dir, 0777)
	}

	return true, runner.InitObjStore(ctx, dir, size, removedC, errorC)
}

func runObjCache(ctx context.Context) (err errors.Error) {

	removedC := make(chan os.FileInfo, 1)
	errorC := make(chan errors.Error, 3)

	go func() {
		defer func() {
			defer func() {
				recover()
			}()

			close(errorC)
			close(removedC)
			if *objCacheCreate {
				os.RemoveAll(*objCacheOpt)
			}
		}()
		for {
			select {
			case err := <-errorC:
				if err != nil {
					logger.Info(err.Error())
				}
			case removed := <-removedC:
				if removed != nil {
					logger.Info(fmt.Sprintf("removed %#v from cache", removed.Name()))
				}
			case <-ctx.Done():
				logger.Warn("cache service stopped")
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
				recover()
			}()
			close(removedC)
		}()

		return nil
	}

	return nil
}
