package main

import (
	"context"
	"os"
	"regexp"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"
	"github.com/go-stack/stack"
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

	// NewRabbitMQ will strip off the user name and password if they appear in
	// the first URL but will preserve them inside the second parameter
	rmq, err := runner.NewRabbitMQ(*amqpURL, *amqpURL)
	if err != nil {
		logger.Error(err.Error())
	}

	// The regular expression is validated in the main.go file
	matcher, _ := regexp.Compile(*queueMatch)

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

			found, err := rmq.GetKnown(matcher, connTimeout)
			if err != nil {
				logger.Warn("unable to refresh RMQ manifest", err.Error())
				qCheck = qCheck * 2
			}
			if len(found) == 0 {
				logger.Warn("no queues found", "uri", rmq.SafeURL, "stack", stack.Trace().TrimRuntime())
				qCheck = qCheck * 2
				continue
			}

			live.Lifecycle(ctx, found)
		}
	}
}
