package main

import (
	"context"
	"fmt"
	"os"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/go-stack/stack"

	"github.com/karlmutch/errors"
)

var (
	listeners *runner.Listeners
)

func k8sStateUpdates() (l *runner.Listeners) {
	return listeners
}

func initiateK8s(ctx context.Context, namespace string, cfgMap string, errorC chan errors.Error) (err errors.Error) {

	listeners = runner.NewStateBroadcast(ctx, errorC)

	// Watch for k8s events that are of interest
	go runner.MonitorK8s(ctx, errorC)

	// If the user did specify the k8s parameters then we need to process the k8s configs
	if len(*cfgNamespace) != 0 && len(*cfgConfigMap) != 0 {
		// If k8s is not running then dont continue and dont signal an error as this
		// mode is supported
		if err = runner.IsAliveK8s(); err != nil {
			logger.Warn("k8s is not alive, ignoring k8s configMap support for state management")
			return nil
		}

		// Start a logger for catching the state changes and printing them
		go k8sStateLogger(ctx)

		// The convention exists that the per machine configmap name is simply the hostname
		podMap := os.Getenv("HOSTNAME")

		// If k8s is specified we need to start a listener for lifecycle
		// states being set in the k8s config map or within a config map
		// that matches our pod/hostname
		if err = runner.ListenK8s(ctx, *cfgNamespace, *cfgConfigMap, podMap, listeners.Master, errorC); err != nil {
			fmt.Println(errors.Wrap(err).With("stack", stack.Trace().TrimRuntime()).Error())
			return err
		}
	}
	return nil
}

func k8sStateLogger(ctx context.Context) {
	listener := make(chan runner.K8sStateUpdate, 1)

	id, err := listeners.Add(listener)

	if err != nil {
		logger.Warn(err.Error())
		return
	}

	defer func() {
		logger.Warn("stopping k8sStateLogger")
		listeners.Delete(id)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case state := <-listener:
			logger.Info("server state is "+state.State.String(), "stack", stack.Trace().TrimRuntime())
		}
	}
}
