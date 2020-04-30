// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/prometheus/client_golang/prometheus"
)

// This file contains the implementation of message handling function used for calling the processor when
// queue specific implementations of message receivers get traffic

// HandleMsg takes a message describing a queued task and handles the request, running and validating it
// in a blocking fashion.  This function will typically be initiated via the queue implementation
// Work(...) method.  The queue implementation Work(...) method will typically be invoked from the
// doWork(...) method of the Queuer receiver.
//
func HandleMsg(ctx context.Context, qt *runner.QueueTask) (rsc *runner.Resource, consume bool, err kv.Error) {

	rsc = nil

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("%#v", r), "stack", stack.Trace().TrimRuntime())
		}
	}()

	// allocate the processor and sub the subscription as
	// the group mechanism for work coming down the
	// pipe that is sent to the resource allocation
	// module
	proc, hardError, err := newProcessor(ctx, qt.Subscription, qt.Msg, qt.Credentials, qt.Wrapper)
	if err != nil {
		return rsc, hardError, err.With("hardErr", hardError)
	}
	defer proc.Close()

	rsc = proc.Request.Experiment.Resource.Clone()

	labels := prometheus.Labels{
		"host":       host,
		"queue_type": "rmq",
		"queue_name": qt.Project + qt.Subscription,
		"project":    proc.Request.Config.Database.ProjectId,
		"experiment": proc.Request.Experiment.Key,
	}

	// Modify the prometheus metrics that track running jobs
	queueRunning.With(labels).Inc()

	startTime := time.Now()
	logger.Debug("experiment started", "experiment_id", proc.Request.Experiment.Key,
		"project_id", proc.Request.Config.Database.ProjectId, "root_dir", proc.RootDir,
		"subscription", qt.Subscription)

	defer func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Info("unable to update counters", "recover", fmt.Sprint(r), "stack", stack.Trace().TrimRuntime())
			}
		}()
		logger.Debug("experiment completed", "duration", time.Since(startTime).String(),
			"experiment_id", proc.Request.Experiment.Key,
			"project_id", proc.Request.Config.Database.ProjectId, "root_dir", proc.RootDir,
			"subscription", qt.Subscription)

		queueRunning.With(labels).Dec()
		queueRan.With(labels).Inc()
	}()

	// Blocking call to run the entire task and only return on termination due to the context
	// being canceled or its own error / success
	ack, err := proc.Process(ctx)
	if err != nil {

		if !ack {
			return rsc, ack, err.With("status", "retry")
		}

		return rsc, ack, err.With("status", "dump")
	}

	return rsc, ack, nil
}
