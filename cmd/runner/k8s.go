package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"
	"github.com/go-stack/stack"

	"github.com/rs/xid"

	"github.com/karlmutch/errors"
)

var (
	listeners = Listeners{
		listeners: map[xid.ID]chan<- types.K8sState{},
	}
)

type Listeners struct {
	listeners map[xid.ID]chan<- types.K8sState
	sync.Mutex
}

func initiateK8s(ctx context.Context, namespace string, cfgMap string, errorC chan errors.Error) (err errors.Error) {

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

		master := make(chan types.K8sState, 1)

		// The convention exists that the per machine configmap name is simply the hostname
		podMap := os.Getenv("HOSTNAME")

		// If k8s is specified we need to start a listener for lifecycle
		// states being set in the k8s config map or within a config map
		// that matches our pod/hostname
		if err = runner.ListenK8s(ctx, *cfgNamespace, *cfgConfigMap, podMap, master, errorC); err != nil {
			fmt.Println(errors.Wrap(err).With("stack", stack.Trace().TrimRuntime()).Error())
			return err
		}

		// Add a listener for the master channel and then perform a fanout to any listeners
		go propogateLifecycle(ctx, master, errorC)
	}
	return nil
}

func propogateLifecycle(ctx context.Context, master chan types.K8sState, errorC chan errors.Error) {
	for {
		select {
		case <-ctx.Done():
			return
		case state := <-master:

			logger.Debug(fmt.Sprint("State fired for ", len(listeners.listeners), " clients"))

			clients := make([]chan<- types.K8sState, 0, len(listeners.listeners))

			// Make a consistent copy of all the channels that the update will be sent down
			// so that we retain the values at this moment in time
			listeners.Lock()
			for _, v := range listeners.listeners {
				clients = append(clients, v)
			}
			listeners.Unlock()

			for _, c := range clients {
				select {
				case c <- state:
				case <-time.After(500 * time.Millisecond):
					logger.Warn("k8s state failed to fire")
				}
			}
		}
	}
}

func k8sStateLogger(ctx context.Context) {
	listener := make(chan types.K8sState, 1)

	id, err := addLifecycleListener(listener)

	if err != nil {
		logger.Warn(err.Error())
		return
	}

	defer func() {
		logger.Warn("stopping k8sStateLogger")
		deleteLifecycleListener(id)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case state := <-listener:
			logger.Info(state.String())
		}
	}
}

func addLifecycleListener(listener chan<- types.K8sState) (id xid.ID, err errors.Error) {

	id = xid.New()
	listeners.Lock()
	defer listeners.Unlock()

	listeners.listeners[id] = listener

	return id, nil
}

func deleteLifecycleListener(id xid.ID) {

	listeners.Lock()
	defer listeners.Unlock()

	delete(listeners.listeners, id)
}
