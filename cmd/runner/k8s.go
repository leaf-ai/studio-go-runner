// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"os"
	"time"

	"github.com/go-stack/stack"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/jjeffery/kv" // MIT License
)

var (
	listeners *runner.Listeners
)

func k8sStateUpdates() (l *runner.Listeners) {
	return listeners
}

// initiateK8s runs until either ctx is Done or the listener is running successfully
func initiateK8s(ctx context.Context, namespace string, cfgMap string, errorC chan kv.Error) {

	// If the user did specify the k8s parameters then we need to process the k8s configs
	if len(*cfgNamespace) == 0 || len(*cfgConfigMap) == 0 {
		return
	}

	listeners = runner.NewStateBroadcast(ctx, errorC)

	// Watch for k8s API connectivity events that are of interest and use the errorC to surface them
	go runner.MonitorK8s(ctx, errorC)

	// Start a logger for catching the state changes and printing them
	go k8sStateLogger(ctx)

	// The convention exists that the per machine configmap name is simply the hostname
	podMap := os.Getenv("HOSTNAME")

	for {
		// If k8s is specified we need to start a listener for lifecycle
		// states being set in the k8s config map or within a config map
		// that matches our pod/hostname
		if err := runner.ListenK8s(ctx, *cfgNamespace, *cfgConfigMap, podMap, listeners.Master, errorC); err != nil {
			logger.Warn("k8s monitoring offline", "error", err.Error())
		}
		<-time.After(30 * time.Second)
	}
}

func k8sStateLogger(ctx context.Context) {
	logger.Info("k8sStateLogger starting")

	listener := make(chan runner.K8sStateUpdate, 1)

	id, err := listeners.Add(listener)

	if err != nil {
		logger.Warn(err.Error())
		return
	}

	defer func() {
		logger.Warn("k8sStateLogger stopping")
		listeners.Delete(id)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case state := <-listener:
			logger.Info("k8s state is "+state.State.String(), "stack", stack.Trace().TrimRuntime())
		}
	}
}
