// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/types"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus"
)

// This file contains the implementation of a RabbitMQ service for
// retriving and handling StudioML workloads within a self hosted
// queue context

func serviceRMQ(ctx context.Context, checkInterval time.Duration, connTimeout time.Duration) {

	logger.Debug("starting serviceRMQ", stack.Trace().TrimRuntime())
	defer logger.Debug("stopping serviceRMQ", stack.Trace().TrimRuntime())

	if len(*amqpURL) == 0 {
		logger.Info("rabbitMQ services disabled", stack.Trace().TrimRuntime())
		return
	}

	live := &Projects{
		queueType: "rabbitMQ",
		projects:  map[string]context.CancelFunc{},
	}

	// NewRabbitMQ takes a URL that has no credentials or tokens attached as the
	// first parameter and the user name password as the second parameter
	creds := ""
	qURL, errGo := url.Parse(os.ExpandEnv(*amqpURL))
	if errGo != nil {
		logger.Warn(kv.Wrap(errGo).With("url", *amqpURL).With("stack", stack.Trace().TrimRuntime()).Error())
	}
	if qURL.User != nil {
		creds = qURL.User.String()
	} else {
		logger.Warn(kv.NewError("missing credentials in url").With("url", *amqpURL).With("stack", stack.Trace().TrimRuntime()).Error())
	}
	qURL.User = nil
	rmq, err := runner.NewRabbitMQ(qURL.String(), creds)
	if err != nil {
		logger.Error(err.Error())
	}

	// The regular expression is validated in the main.go file
	matcher, errGo := regexp.Compile(*queueMatch)
	if errGo != nil {
		if len(*queueMatch) != 0 {
			logger.Warn(kv.Wrap(errGo).With("matcher", *queueMatch).With("stack", stack.Trace().TrimRuntime()).Error())
		}
		matcher = nil
	}

	// If the length of the mismatcher is 0 then we will get a nil and because this
	// was checked in the main we can ignore that as this is optional
	mismatcher := &regexp.Regexp{}

	if len(strings.Trim(*queueMismatch, " \n\r\t")) == 0 {
		mismatcher = nil
	} else {
		mismatcher, errGo = regexp.Compile(*queueMismatch)
		if errGo != nil {
			if len(*queueMismatch) != 0 {
				logger.Warn(kv.Wrap(errGo).With("mismatcher", *queueMismatch).With("stack", stack.Trace().TrimRuntime()).Error())
			}
			mismatcher = nil
		}
	}

	// first time through make sure the credentials are checked immediately
	qCheck := time.Duration(time.Second)

	// Watch for when the server should not be getting new work
	state := runner.K8sStateUpdate{
		State: types.K8sRunning,
	}

	lifecycleC := make(chan runner.K8sStateUpdate, 1)
	id, err := k8sStateUpdates().Add(lifecycleC)
	defer func() {
		k8sStateUpdates().Delete(id)
		close(lifecycleC)
	}()

	host, errGo := os.Hostname()
	if errGo != nil {
		logger.Warn(errGo.Error())
	}

	for {
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
			return
		case state = <-lifecycleC:
		case <-time.After(qCheck):
			qCheck = checkInterval

			// If the pulling of work is currently suspending bail out of checking the queues
			if state.State != types.K8sRunning {
				queueIgnored.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": "*"}).Inc()
				logger.Debug("k8s has RMQ disabled", "stack", stack.Trace().TrimRuntime())
				continue
			}

			// Intentional shadowing with ctx
			ctx, cancel := context.WithTimeout(ctx, connTimeout)

			// Found returns a map that contains the queues that were found
			// on the rabbitMQ server specified by the rmq data structure
			found, err := rmq.GetKnown(ctx, matcher, mismatcher)
			cancel()

			if err != nil {
				logger.Warn("unable to refresh RMQ manifest", err.Error())
				qCheck = qCheck * 2
				continue
			}
			if len(found) == 0 {
				items := []string{"no queues found", "identity", rmq.Identity, "matcher", matcher.String()}

				if mismatcher != nil {
					items = append(items, "mismatcher", mismatcher.String())
				}
				items = append(items, "stack", stack.Trace().TrimRuntime().String())
				logger.Warn(items[0], items[1:])

				qCheck = qCheck * 2
				continue
			}

			// found contains a map of keys that have an uncredentialed URL, and the value which is the user name and password for the URL
			//
			// The URL path is going to be the vhost and the queue name
			live.Lifecycle(ctx, found)
		}
	}
}
