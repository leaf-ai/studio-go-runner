// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/karlmutch/base62"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/pkg/studio"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/golang/protobuf/ptypes/wrappers"
	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file contains the implementation of message handling function used for calling the processor when
// queue specific implementations of message receivers get traffic

// HandleMsg takes a message describing a queued task and handles the request, running and validating it
// in a blocking fashion.  This function will typically be initiated via the queue implementation
// Work(...) method.  The queue implementation Work(...) method will typically be invoked from the
// doWork(...) method of the Queuer receiver.
//
func HandleMsg(ctx context.Context, qt *runner.QueueTask) (rsc *studio.Resource, consume bool, err kv.Error) {

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("%#v", r), "stack", stack.Trace().TrimRuntime())
		}
	}()

	host = studio.GetHostName()
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
		queueRunning.With(labels).Dec()
		queueRan.With(labels).Inc()

		logger.Debug("experiment completed", "duration", time.Since(startTime).String(),
			"experiment_id", proc.Request.Experiment.Key,
			"project_id", proc.Request.Config.Database.ProjectId, "root_dir", proc.RootDir,
			"subscription", qt.Subscription)
	}()

	if qt.ResponseQ != nil {
		select {
		case qt.ResponseQ <- &runnerReports.Report{
			Time: timestamppb.Now(),
			ExecutorId: &wrappers.StringValue{
				Value: studio.GetHostName(),
			},
			UniqueId: &wrappers.StringValue{
				Value: accessionID,
			},
			ExperimentId: &wrappers.StringValue{
				Value: proc.Request.Experiment.Key,
			},
			Payload: &runnerReports.Report_Progress{
				Progress: &runnerReports.Progress{
					Time:  timestamppb.Now(),
					State: runnerReports.TaskState_Started,
				},
			},
		}:
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

	if qt.ResponseQ != nil {
		errDetails := &runnerReports.Progress_Error{}
		state := runnerReports.TaskState_Success
		if err != nil {
			state = runnerReports.TaskState_Failed
			errDetails.Msg = &wrappers.StringValue{
				Value: err.Error(),
			}
		}
		select {
		case qt.ResponseQ <- &runnerReports.Report{
			Time: timestamppb.Now(),
			ExecutorId: &wrappers.StringValue{
				Value: studio.GetHostName(),
			},
			UniqueId: &wrappers.StringValue{
				Value: accessionID,
			},
			ExperimentId: &wrappers.StringValue{
				Value: proc.Request.Experiment.Key,
			},
			Payload: &runnerReports.Report_Progress{
				Progress: &runnerReports.Progress{
					Time:  timestamppb.Now(),
					State: state,
					Error: errDetails,
				},
			},
		}:
		default:
			// If this queue backs up dont response to failures
			// as back preassure is a sign on something very wrong
			// that we cannot correct
		}
	}
	return rsc, ack, nil
}
