// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a set of Kubernetes
// functionality tests

import (
	"context"
	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/andreidenissov-cog/go-service/pkg/types"
	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"net/http"
	"os"
	"testing"

	"github.com/karlmutch/k8s"
	core "github.com/karlmutch/k8s/apis/core/v1"
	meta "github.com/karlmutch/k8s/apis/meta/v1"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// setNamedState will change the state parameter in a named config map within the
// current pod namespace
//
func setNamedState(ctx context.Context, name string, namespace string, state types.K8sState) (err kv.Error) {
	// K8s API receiver to be used to manipulate the config maps we are testing
	client, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	configMap := &core.ConfigMap{
		Metadata: &meta.ObjectMeta{
			Name:      k8s.String(name),
			Namespace: k8s.String(namespace),
		},
		Data: map[string]string{"STATE": state.String()},
	}

	// Go and create a k8s config map that we can use for testing purposes
	if errGo = client.Update(ctx, configMap); errGo != nil {
		// If an HTTP error was returned by the API server, it will be of type
		// *k8s.APIError. This can be used to inspect the status code.
		if apiErr, ok := errGo.(*k8s.APIError); ok {
			// Resource already exists. Carry on.
			if apiErr.Code == http.StatusNotFound {
				errGo = client.Create(ctx, configMap)
			}
		}
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}

	return nil
}

// setGlobalState is used to modify the globally used k8s state configmap
func setGlobalState(ctx context.Context, namespace string, state types.K8sState) (err kv.Error) {
	return setNamedState(ctx, "studioml-go-runner", namespace, state)
}

// setLocalState is used to modify the node specific state configmap
func setLocalState(ctx context.Context, namespace string, state types.K8sState) (err kv.Error) {
	host, errGo := os.Hostname()
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return setNamedState(ctx, host, namespace, state)
}

// Test0InitK8s is used to validate the test environments secrets for
// message encryption if Kubernetes is present
//
func Test0InitK8s(t *testing.T) {

	if err := server.IsAliveK8s(); err != nil && !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	if err := server.IsAliveK8s(); err != nil {
		t.Fatal(err)
	}
	w, err := defense.KubernetesWrapper(*msgEncryptDirOpt)
	if err != nil {
		t.Fatal(err)
	}

	// If kubernetes is present there MUST be secrets loaded to run message encryption
	if w == nil {
		t.Fatal(kv.NewError("wrapper missing").With("stack", stack.Trace().TrimRuntime()))
	}
}

// TestK8sConfigNode is used to test that both the global and the node specific config
// map changes within Kubernetes are observed by the runner.  This is a live test that
// exercises the k8s functionality as well as the runners internal listener
// functionality.
//
//func TestK8sConfigNode(t *testing.T) {
//
//	if err := server.IsAliveK8s(); err != nil && !*useK8s {
//		t.Skip("kubernetes specific testing disabled")
//	}
//
//	if err := server.IsAliveK8s(); err != nil {
//		t.Fatal(err)
//	}
//
//	// The downward API within K8s is configured within the build YAML
//	// to pass the pods namespace into the pods environment table, it will be named
//	// appropriately for the command line argument names being used by the runner
//	namespace := *cfgNamespace
//
//	// When the watch sees a state change it will attempt to wake up receiver on a channel,
//	// which in this case will be the test waiting for a state to be applied by the runner
//	// under test
//	wakeupC := make(chan struct{}, 1)
//	defer close(wakeupC)
//
//	// To test config map access we extract the host name and the namespace we are running
//	// in then we create the system wide and node specific config maps if they are not
//	// already present
//	//
//	state := types.K8sUnknown
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	// Start out by registering a listener for state changes and when one
//	// arrives change the variable, state so that the test can validate results
//	go func(ctx context.Context) {
//
//		stateC := make(chan server.K8sStateUpdate, 1)
//		defer close(stateC)
//
//		id, err := server.K8sStateUpdates().Add(stateC)
//		if err != nil {
//			logger.Fatal(err.Error())
//			return
//		}
//		defer server.K8sStateUpdates().Delete(id)
//
//		for {
//			select {
//			case <-ctx.Done():
//				return
//			case update := <-stateC:
//				state = update.State
//				// Wakeup the test waiting on state changes, if a message is
//				// already queued up then abandon the send
//				select {
//				case wakeupC <- struct{}{}:
//				default:
//				}
//			}
//		}
//	}(ctx)
//
//	// Watch the states every second to see when they change
//	sampleState := time.NewTicker(time.Second)
//	defer sampleState.Stop()
//
//	// Set the global state to an initial good value and dont wait for the refresh to occur
//	// as the first state change might not propagate through as it might not be a change
//	// and the refresh that is done within the runner is not in the order of single digit
//	// seconds
//	if err := setGlobalState(ctx, namespace, types.K8sDrainAndSuspend); err != nil {
//		t.Fatal(err)
//	}
//
//	// Set the global state to running, which should flip the previous state we just set
//	// and result in an update in the state channel
//	if err := setGlobalState(ctx, namespace, types.K8sRunning); err != nil {
//		t.Fatal(err)
//	}
//
//	// Check for the state to change to the correct globally signalled state we just set
//	deadline := time.Now().Add(5 * time.Second)
//	for {
//		select {
//		case <-sampleState.C:
//		case <-wakeupC:
//		}
//		if state == types.K8sRunning {
//			break
//		}
//		if time.Now().After(deadline) {
//			t.Fatal("Global running state was not updated in time", namespace, stack.Trace().TrimRuntime())
//		}
//	}
//
//	// Set the node state to DrainAndSuspend
//	if err := setLocalState(ctx, namespace, types.K8sDrainAndSuspend); err != nil {
//		t.Fatal(err)
//	}
//
//	// Check for the state to change to the correct locally signalled state we just set
//	deadline = time.Now().Add(5 * time.Second)
//	for {
//		select {
//		case <-sampleState.C:
//		case <-wakeupC:
//		}
//		if state == types.K8sDrainAndSuspend {
//			break
//		}
//		if time.Now().After(deadline) {
//			t.Fatal("Local drain and suspend state was not updated in time", stack.Trace().TrimRuntime())
//		}
//	}
//
//	// Set the node state to Running
//	if err := setLocalState(ctx, namespace, types.K8sRunning); err != nil {
//		t.Fatal(err)
//	}
//
//	// Check for the state to change to the correct locally signalled state we just set
//	deadline = time.Now().Add(5 * time.Second)
//	for {
//		select {
//		case <-sampleState.C:
//		case <-wakeupC:
//		}
//		if state == types.K8sRunning {
//			break
//		}
//		if time.Now().After(deadline) {
//			t.Fatal("Local running state was not updated in time", stack.Trace().TrimRuntime())
//		}
//	}
//}
