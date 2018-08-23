package main

import (
	"context"
	"sync"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"

	"github.com/rs/xid"

	"github.com/karlmutch/errors"
)

var (
	masterListen <-chan types.K8sState // Updates to states will be sent to this single channel then fanned out

	listeners = Listeners{
		listeners: map[xid.ID]chan types.K8sState{},
	}
)

type Listeners struct {
	listeners map[xid.ID]chan types.K8sState
	sync.Mutex
}

func initiateK8s(quitCtx context.Context, namespace string, cfgMap string, errorC chan errors.Error) (err errors.Error) {

	// Watch for k8s events that are of interest
	go runner.MonitorK8s(quitCtx, errorC)

	// Add a listener for the master channel and then perform a fanout to any listeners
	go propogateLifecycle(quitCtx, errorC)

	// If the user did specify the k8s parameters then we need to process the k8s configs
	if len(*cfgNamespace) != 0 && len(*cfgConfigMap) != 0 {
		if err = runner.IsAliveK8s(); err != nil {
			return err
		}

		cfgs, err := runner.ConfigK8s(quitCtx, *cfgNamespace, *cfgConfigMap)
		if err != nil {
			return err
		}
		Spew.Dump(cfgs)

		// If k8s is specified we need to start a listener for lifecycle
		// states being set in the k8s config map or within a config map
		// that matches our pod/hostname
		masterListen = runner.ListenK8s(quitCtx, *cfgNamespace, errorC)
	}
	return nil
}

func propogateLifecycle(quitCtx context.Context, errorC chan errors.Error) {
	for {
		select {
		case <-quitCtx.Done():
			return
		case state := <-masterListen:
			clients := make([]chan<- types.K8sState, 0, len(listeners.listeners))
			// Make a consistent copy of all the channels that the update will be sent down
			// so that we retain the values at this moment in time
			func() {
				listeners.Lock()
				defer listeners.Unlock()
				for _, v := range listeners.listeners {
					clients = append(clients, v)
				}
			}()
			for _, c := range clients {
				select {
				case c <- state:
				case <-time.After(500 * time.Millisecond):
				}
			}
		}
	}
}

func addLifecycleListener(listener chan<- types.K8sState) (id xid.ID, err errors.Error) {

	id = xid.New()
	listeners.Lock()
	defer listeners.Unlock()

	listeners.listeners[id] = make(chan types.K8sState, 1)

	return id, nil
}

func deleteLifecycleListener(id xid.ID) {

	listeners.Lock()
	defer listeners.Unlock()

	delete(listeners.listeners, id)
}
