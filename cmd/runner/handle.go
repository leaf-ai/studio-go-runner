// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/karlmutch/base62"

	"github.com/leaf-ai/go-service/pkg/network"
	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/task"
)

var (
	allowEnvSecrets = flag.Bool("allow-env-secrets", false, "allow the use of environment variables, such as AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, for global artifact credential defaults")
)

// This file contains the implementation of message handling function used for calling the processor when
// queue specific implementations of message receivers get traffic

// HandleMsg takes a message describing a queued task and handles the request, running and validating it
// in a blocking fashion.  This function will typically be initiated via the queue implementation
// Work(...) method.  The queue implementation Work(...) method will typically be invoked from the
// doWork(...) method of the Queuer receiver.
func HandleMsg(ctx context.Context, qt *task.QueueTask) (rsc *server.Resource, consume bool, err kv.Error) {

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("%#v", r), "stack", stack.Trace().TrimRuntime())
		}
	}()

	host = network.GetHostName()
	accessionID := host + "-" + base62.EncodeInt64(time.Now().Unix())

	// allocate the processor and use the subscription name as the group by for work coming down the
	// pipe that is sent to the resource allocation module
	proc, hardError, err := newProcessor(ctx, qt, accessionID)
	if proc != nil {
		rsc = proc.Request.Experiment.Resource.Clone()
		if rsc == nil {
			logger.Warn("resource spec empty", "subscription", qt.Subscription, "stack", stack.Trace().TrimRuntime())
		}
		defer proc.Close()
	}

	if err != nil {
		return rsc, hardError, err.With("hardErr", hardError)
	}

	// Check for the presence of artifact credentials and if we see none, then for backward
	// compatibility, see if there are AWS credentials in the env variables and if so load these
	// into the artifacts
	for key, art := range proc.Request.Experiment.Artifacts {
		if art.Credentials.Plain != nil {
			continue
		}
		if art.Credentials.JWT != nil {
			continue
		}
		if art.Credentials.AWS != nil {
			continue
		}
		if *allowEnvSecrets {
			if accessKey, isPresent := proc.Request.Config.Env["AWS_ACCESS_KEY_ID"]; isPresent {
				secretKey := proc.Request.Config.Env["AWS_SECRET_ACCESS_KEY"]
				newArt := art.Clone()
				newArt.Credentials = request.Credentials{
					AWS: &request.AWSCredential{
						AccessKey: accessKey,
						SecretKey: secretKey,
					},
				}
				proc.Request.Experiment.Artifacts[key] = *newArt
			}
		}
	}

	// Modify the prometheus metrics that track running jobs
	atomic.AddInt32(&queueRunning, 1)

	startTime := time.Now()

	defer func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Info("unable to update counters", "recover", fmt.Sprint(r), "stack", stack.Trace().TrimRuntime())
			}
		}()
		atomic.AddInt32(&queueRunning, -1)
		atomic.AddInt32(&queueRan, 1)

		logger.Debug("experiment completed", "duration", time.Since(startTime).String(),
			"experiment_id", proc.Request.Experiment.Key,
			"project_id", proc.Request.Config.Database.ProjectId, "root_dir", proc.RootDir,
			"subscription", qt.Subscription)
	}()

	logger.Debug("experiment started", "experiment_id", proc.Request.Experiment.Key,
		"project_id", proc.Request.Config.Database.ProjectId, "root_dir", proc.RootDir,
		"subscription", qt.Subscription)

	if qt.ResponseQ != nil {
		select {
		case qt.ResponseQ <- "":
		default:
			// If this queue backs up dont response to failures
			// as back preassure is a sign on something very wrong
			// that we cannot correct
		}
	}

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
