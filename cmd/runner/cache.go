package main

// The file contains the implementation of functions related to starting and maintaining a
// disk cache for the artifacts being used by the runner

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/SentientTechnologies/studio-go-runner"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	objCacheOpt    = flag.String("cache-dir", "", "An optional directory to be used as a cache for downloaded artifacts")
	objCacheMaxOpt = flag.String("cache-size", "", "The maximum target size of the disk based download cache")
)

func getCacheOptions() (dir string, size string, err errors.Error) {
	if len(*objCacheOpt) != 0 || len(*objCacheMaxOpt) != 0 {
		if len(*objCacheOpt) == 0 {
			return dir, size, errors.Wrap(fmt.Errorf("if the option cache-size is specified the cache-dir must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
		if len(*objCacheMaxOpt) == 0 {
			return dir, size, errors.Wrap(fmt.Errorf("if the option cache-dir is specified the cache-size must also be specified")).With("stack", stack.Trace().TrimRuntime())
		}
		if _, errGo := humanize.ParseBytes(*objCacheMaxOpt); errGo != nil {
			return dir, size, errors.Wrap(errGo, "option cache-size was not formatted correctly").With("stack", stack.Trace().TrimRuntime())
		}
	}
	return *objCacheOpt, *objCacheMaxOpt, nil
}

func startObjStore(removedC chan os.FileInfo, errorC chan errors.Error, quitC chan bool) (err errors.Error) {
	dir, size, err := getCacheOptions()
	if err != nil {
		return err
	}

	return runner.InitObjStore(dir, size, removedC, errorC, quitC)
}

func runObjCache(ctx context.Context) (err errors.Error) {

	removedC := make(chan os.FileInfo)
	errorC := make(chan errors.Error)
	quitC := make(chan bool)

	if err = startObjStore(removedC, errorC, quitC); err != nil {
		return err
	}

	go func() {
		defer close(quitC)
		defer close(removedC)
		defer close(errorC)
		for {
			select {
			case err := <-errorC:
				logger.Info(fmt.Sprintf("cache error %#v", err))
			case removed := <-removedC:
				logger.Info(fmt.Sprintf("removed %#v from cache", removed.Name()))
			case <-ctx.Done():
				return
			}
		}
	}()
	return nil
}
