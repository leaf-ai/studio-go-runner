package runner

// This file contains functions related to Kubernetes (k8s) support for the runner.
// The runner can use k8s to watch and load ConfigMap information that it can use
// to manage its life cycle and in the future to load configuration information.
//
// The choice to make use of the package from Eric Chiang is driven by the
// package dependency issues with using the official go client.  It rivals
// the spagetti dependencies of Dockers buildkit, borderline horrific.  The choosen
// package has a single dependency and trades off using generated protobuf structures
// and so it wired to the k8s versions via that method, a tradeoff I'm willing to
// make based on my attempts with BuildKit.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/ericchiang/k8s"
	core "github.com/ericchiang/k8s/apis/core/v1"
	"github.com/go-stack/stack"
	"github.com/lthibault/jitterbug"

	"github.com/karlmutch/errors"

	"github.com/SentientTechnologies/studio-go-runner/internal/types"
)

var (
	k8sClient  *k8s.Client
	k8sInitErr errors.Error

	protect sync.Mutex
)

func init() {
	protect.Lock()
	defer protect.Unlock()

	client, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		k8sInitErr = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		return
	}
	k8sClient = client
}

// MonitorK8s is used to initiate k8s connectivity and check if we
// are running within a cluster
//
func MonitorK8s(ctx context.Context, errC chan<- errors.Error) {

	t := jitterbug.New(time.Second*30, &jitterbug.Norm{Stdev: time.Second * 3})
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-t.C:
			func() {
				protect.Lock()
				defer protect.Unlock()
				if k8sClient == nil && k8sInitErr != nil {
					select {
					case errC <- k8sInitErr:
						return
					default:
					}
				} else {
					if k8sClient != nil && k8sInitErr == nil {
						return
					}
				}
			}()
		}
	}
}

// IsAliveK8s is used to extract any errors in the state of the k8s client api connection.
//
// A nil returned indicates k8s is working and in use, otherwise a descriptive error
// is returned.
//
func IsAliveK8s() (err errors.Error) {
	protect.Lock()
	defer protect.Unlock()
	if k8sInitErr != nil {
		return k8sInitErr
	}
	if k8sClient == nil {
		return errors.New("Kubernetes uninitialized or no cluster present").With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// ConfigK8s is used to pull the values from a named config map in k8s
//
func ConfigK8s(ctx context.Context, namespace string, name string) (values map[string]string, err errors.Error) {
	values = map[string]string{}
	if err = IsAliveK8s(); err != nil {
		return values, nil
	}
	cfg := &core.ConfigMap{}

	if errGo := k8sClient.Get(ctx, namespace, name, cfg); errGo != nil {
		return values, errors.Wrap(errGo).With("namespace", namespace).With("name", name).With("stack", stack.Trace().TrimRuntime())
	}

	if name == *cfg.Metadata.Name {
		return cfg.Data, nil
	}
	return values, nil
}

func listenerK8s(ctx context.Context, namespace string, stateC chan types.K8sState, errC chan<- errors.Error) {

	defer close(stateC)

	// Check for a new state set on the k8s configMap, will update every 30
	// seconds but will only propogate states if there is a change
	t := jitterbug.New(time.Second*30, &jitterbug.Norm{Stdev: time.Second * 3})
	defer t.Stop()

	// Once every 3 minutes for so we will force the state propogation
	// to ensure that modules started after this module has started see something
	refresh := jitterbug.New(time.Minute*3, &jitterbug.Norm{Stdev: time.Second * 15})
	defer refresh.Stop()

	currentState := types.K8sUnknown

	dynCfgName := "studioml-" + os.Getenv("HOSTNAME")
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh.C:
			currentState = types.K8sUnknown
		case <-t.C:
			values, err := ConfigK8s(ctx, namespace, dynCfgName)
			if err != nil {
				if apiErr, ok := errors.Cause(err).(*k8s.APIError); ok {
					// ConfigMap not found, which is OK as this is a per pod signal
					if apiErr.Code == http.StatusNotFound {
						continue
					}
				}
				select {
				case errC <- err:
				case <-time.After(2 * time.Second):
				}
				continue
			}
			// Now we have the config map that should contain values
			// which indicate what the state of the runner should be
			stateStr, isPresent := values["STATE"]
			if isPresent {
				newState, errGo := types.K8sStateString(stateStr)
				if errGo != nil {
					msg := errors.Wrap(errGo).With("namespace", namespace).With("config", dynCfgName).With("state", stateStr).With("stack", stack.Trace().TrimRuntime())
					select {
					case errC <- msg:
					case <-time.After(2 * time.Second):
						fmt.Println(err)
					}
				}
				if newState == currentState {
					continue
				}
				// Try sending the new state to listeners within the server invoking this function
				select {
				case stateC <- newState:
					currentState = newState
				case <-time.After(time.Second):
					continue
				}
			}
		}
	}
}

// ListenK8s will register a listener to watch for pod specific configMaps in k8s
// and will relay state changes to a channel
func ListenK8s(ctx context.Context, namespace string, errC chan<- errors.Error) (stateC <-chan types.K8sState) {

	updateC := make(chan types.K8sState, 1)

	go listenerK8s(ctx, namespace, updateC, errC)

	return updateC
}
