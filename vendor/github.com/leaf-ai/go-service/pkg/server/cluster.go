// Copyright 2018-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package server // import "github.com/leaf-ai/go-service/pkg/server"

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/leaf-ai/go-service/pkg/log"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	listeners *Listeners
)

func K8sStateUpdates() (l *Listeners) {
	return listeners
}

// initiateK8s runs until either ctx is Done or the listener is running successfully
func InitiateK8s(ctx context.Context, namespace string, cfgMap string, readyC chan struct{}, staleMsg time.Duration, logger *log.Logger, errorC chan kv.Error) {

	// If the user did specify the k8s parameters then we need to process the k8s configs
	if len(namespace) == 0 || len(cfgMap) == 0 {
		return
	}

	listeners = NewStateBroadcast(ctx, errorC)

	func() {
		defer recover()
		close(readyC)
	}()

	// Watch for k8s API connectivity events that are of interest and use the errorC to surface them
	go MonitorK8s(ctx, errorC)

	// Start a logger for catching the state changes and printing them
	go k8sStateLogger(ctx, staleMsg, logger)

	// The convention exists that the per machine configmap name is simply the hostname
	podMap := os.Getenv("HOSTNAME")

	// In the event that initializing the k8s listener fails we try once every 30 seconds to get it working
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			// If k8s is specified we need to start a listener for lifecycle
			// states being set in the k8s config map or within a config map
			// that matches our pod/hostname
			if err := ListenK8s(ctx, namespace, cfgMap, podMap, listeners.Master, errorC); err != nil {
				logger.Warn("k8s monitoring offline", "error", err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func k8sStateLogger(ctx context.Context, refreshMsg time.Duration, logger *log.Logger) {
	logger.Info("k8sStateLogger starting")

	listener := make(chan K8sStateUpdate, 1)

	id, err := listeners.Add(listener)

	if err != nil {
		logger.Warn(err.Error())
		return
	}

	defer func() {
		logger.Warn("k8sStateLogger stopping")
		listeners.Delete(id)
	}()

	lastMsg := ""
	nextTime := time.Now().Add(refreshMsg)

	for {
		select {
		case <-ctx.Done():
			return
		case state := <-listener:
			msg := fmt.Sprint("k8s state is "+state.State.String(), "stack", stack.Trace().TrimRuntime())
			if msg == lastMsg {
				if nextTime.Before(time.Now()) {
					continue
				}
				nextTime = time.Now().Add(refreshMsg)

			} else {
				lastMsg = msg
				nextTime = time.Now().Add(refreshMsg)
			}
			logger.Info(msg)
		}
	}
}
