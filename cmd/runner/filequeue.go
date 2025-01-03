// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/go-service/pkg/types"

	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/go-stack/stack"
)

// This file contains the implementation of a RabbitMQ service for
// retrieving and handling StudioML workloads within a self hosted
// queue context

func initFileQueueParams() (matcher *regexp.Regexp, mismatcher *regexp.Regexp) {

	// The regular expression is validated in the main.go file
	matcher, mismatcher = runner.GetQueuePatterns()
	return matcher, mismatcher
}

// serviceFileQueue runs for the lifetime of the daemon and uses the ctx to perform orderly shutdowns.
// This function will initiate checks of the file queue root directories
// for new queues that require processing
// using the projects server Cycle function.
func serviceFileQueue(ctx context.Context, checkInterval time.Duration) {

	logger.Debug("starting serviceFileQueue", stack.Trace().TrimRuntime())
	defer logger.Debug("stopping serviceFileQueue", stack.Trace().TrimRuntime())

	if len(*localQueueRootOpt) == 0 {
		logger.Info("local file queue services disabled", stack.Trace().TrimRuntime())
		return
	}

	matcher, mismatcher := initFileQueueParams()
	fqProject := runner.NewLocalQueue(*localQueueRootOpt, logger)

	// Tracks all known queues and their cancel functions so they can have any
	// running jobs terminated should they disappear
	live := &Projects{
		queueType: "LocalQueue",
		projects:  map[string]context.CancelFunc{},
	}

	defer func() {
		// Ignore failures to cleanup resources we will never reuse
		func() {
			defer func() {
				_ = recover()
			}()
		}()
	}()

	// first time through make sure the credentials are checked immediately
	qCheck := time.Second
	currentCheck := qCheck
	qTicker := time.NewTicker(currentCheck)
	defer qTicker.Stop()

	// Watch for when the server should not be getting new work
	state := server.K8sStateUpdate{
		State: types.K8sRunning,
	}

	for {
		// Dont wait an excessive amount of time after server checks fail before
		// retrying
		if qCheck > time.Duration(3*time.Minute) {
			qCheck = time.Duration(3 * time.Minute)
		}

		// If the interval between queue checks changes reset the ticker
		if qCheck != currentCheck {
			currentCheck = qCheck
			qTicker.Stop()
			qTicker = time.NewTicker(currentCheck)
		}

		select {
		case <-ctx.Done():
			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				if quiter != nil {
					quiter()
				}
			}
			logger.Debug("quitC done for serviceFileQueue", "stack", stack.Trace().TrimRuntime())
			return
		case <-qTicker.C:

			// The user has not specified a local root queue directory which means the
			// local file queue is not needed, we will check back every now and again
			if len(*localQueueRootOpt) == 0 {
				continue
			}

			ran := queueRan
			running := queueRunning

			msg := fmt.Sprintf("checking serviceFileQueue, with %d running tasks and %d completed tasks", running, ran)
			logger.Debug(msg, "stack", stack.Trace().TrimRuntime())

			qCheck = checkInterval

			// If the pulling of work is currently suspending bail out of checking the queues
			if state.State != types.K8sRunning && state.State != types.K8sUnknown {
				logger.Trace("k8s has FileQueue disabled", "stack", stack.Trace().TrimRuntime())
				continue
			}

			// Found returns a map that contains the queues that were found
			// on the file queues root specified by the FileQueue data structure
			found, err := fqProject.GetKnown(ctx, matcher, mismatcher)

			if err != nil {
				qCheck = qCheck * 2
				err = err.With("backoff", qCheck.String())
				logger.Warn("unable to refresh file queues collection", err.Error())
				continue
			}
			if len(found) == 0 {
				items := []string{"no queues", "identity", fqProject.RootDir, "matcher", matcher.String()}

				if mismatcher != nil {
					items = append(items, "mismatcher", mismatcher.String())
				}
				items = append(items, "stack", stack.Trace().TrimRuntime().String())
				logger.Warn(items[0], items[1:])

				qCheck = qCheck * 2
				continue
			}

			// Found needs to just have the main queue servers as their keys, individual queues will be treated as subscriptions
			if err := live.Cycle(ctx, found); err != nil {
				logger.Warn(err.Error())
			}
		}
	}
}
