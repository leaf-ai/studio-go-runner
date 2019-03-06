package runner

// This file contains functions related to Kubernetes (k8s) support for the runner.
// The runner can use k8s to watch and load ConfigMap information that it can use
// to manage its life cycle and in the future to load configuration information.
//
// The choice to make use of the package from Eric Chiang is driven by the
// package dependency issues with using the official go client.  It rivals
// the spagetti dependencies of Dockers buildkit, borderline horrific.  The chosen
// package has a single dependency and trades off using generated protobuf structures
// and so it wired to the k8s versions via that method, a tradeoff I'm willing to
// make based on my attempts with BuildKit.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/ericchiang/k8s"
	core "github.com/ericchiang/k8s/apis/core/v1"

	"github.com/go-stack/stack"
	"github.com/lthibault/jitterbug"

	"github.com/jjeffery/kv" // MIT License

	"github.com/leaf-ai/studio-go-runner/internal/types"
)

var (
	k8sClient  *k8s.Client
	k8sInitErr kv.Error

	protect sync.Mutex
)

func init() {
	protect.Lock()
	defer protect.Unlock()

	client, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		k8sInitErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		return
	}

	k8sClient = client
}

func watchCMaps(ctx context.Context, namespace string) (cmChange chan *core.ConfigMap, err kv.Error) {

	configMap := core.ConfigMap{}
	watcher, errGo := k8sClient.Watch(ctx, namespace, &configMap)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	cmChange = make(chan *core.ConfigMap, 1)
	go func() {

		defer func() {
			if watcher != nil {
				watcher.Close() // Always close the returned watcher.
			}
		}()

		for {
			cm := &core.ConfigMap{}
			// Next does not support cancellation and is block so we have to
			// abandon this thread and simply let it run unmanaged
			_, err := watcher.Next(cm)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				watcher = nil
				// watcher encountered and error, create a new watcher
				watcher, _ = k8sClient.Watch(ctx, namespace, &configMap)
				continue
			}
			select {
			case cmChange <- cm:
			case <-time.After(time.Second):
				spew.Dump(*cm)
			}
		}
	}()
	return cmChange, nil
}

// MonitorK8s is used to initiate k8s connectivity and check if we
// are running within a cluster
//
func MonitorK8s(ctx context.Context, errC chan<- kv.Error) {

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

// IsAliveK8s is used to extract any kv.in the state of the k8s client api connection.
//
// A nil returned indicates k8s is working and in use, otherwise a descriptive error
// is returned.
//
func IsAliveK8s() (err kv.Error) {
	protect.Lock()
	defer protect.Unlock()
	if k8sInitErr != nil {
		fmt.Println(kv.Wrap(k8sInitErr).With("stack", stack.Trace().TrimRuntime()))
		return k8sInitErr
	}
	if k8sClient == nil {
		return kv.NewError("Kubernetes uninitialized or no cluster present").With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// ConfigK8s is used to pull the values from a named config map in k8s
//
// This function will return an empty map and and error value on failure.
//
func ConfigK8s(ctx context.Context, namespace string, name string) (values map[string]string, err kv.Error) {
	values = map[string]string{}

	if err = IsAliveK8s(); err != nil {
		return values, nil
	}
	cfg := &core.ConfigMap{}

	if errGo := k8sClient.Get(ctx, namespace, name, cfg); errGo != nil {
		return values, kv.Wrap(errGo).With("namespace", namespace).With("name", name).With("stack", stack.Trace().TrimRuntime())
	}

	if name == *cfg.Metadata.Name {
		fmt.Println(spew.Sdump(cfg.Data), stack.Trace().TrimRuntime())
		return cfg.Data, nil
	}
	return values, kv.NewError("configMap not found").With("namespace", namespace).With("name", name).With("stack", stack.Trace().TrimRuntime())
}

// K8sStateUpdate encapsulates the known kubernetes state within which the runner finds itself.
//
type K8sStateUpdate struct {
	Name  string
	State types.K8sState
}

// ListenK8s will register a listener to watch for pod specific configMaps in k8s
// and will relay state changes to a channel,  the global state map should exist
// at the bare minimum.  A state change in either map superseded any previous
// state
//
func ListenK8s(ctx context.Context, namespace string, globalMap string, podMap string, updateC chan<- K8sStateUpdate, errC chan<- kv.Error) (err kv.Error) {

	// If k8s is not being used ignore this feature
	if err = IsAliveK8s(); err != nil {
		fmt.Println(kv.Wrap(err).With("stack", stack.Trace().TrimRuntime()).Error())
		return nil
	}

	// Starts the application level state watching
	currentState := K8sStateUpdate{
		State: types.K8sUnknown,
	}

	// Start the k8s configMap watcher
	cmChanges, err := watchCMaps(ctx, namespace)
	if err != nil {
		// The implication of an error here is that we will never get updates from k8s
		return err
	}

	go func(ctx context.Context) {
		// Once every 3 minutes for so we will force the state propagation
		// to ensure that modules started after this module has started see something
		refresh := jitterbug.New(time.Minute*3, &jitterbug.Norm{Stdev: time.Second * 15})
		defer refresh.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-refresh.C:
				// Try resending an existing state to listeners to refresh things
				select {
				case updateC <- currentState:
				case <-time.After(2 * time.Second):
				}
			case cm := <-cmChanges:
				if *cm.Metadata.Namespace == namespace && (*cm.Metadata.Name == globalMap || *cm.Metadata.Name == podMap) {
					if state, _ := cm.Data["STATE"]; len(state) != 0 {
						newState, errGo := types.K8sStateString(state)
						if errGo != nil {
							msg := kv.Wrap(errGo).With("namespace", namespace).With("config", *cm.Metadata.Name).With("state", state).With("stack", stack.Trace().TrimRuntime())
							select {
							case errC <- msg:
							case <-time.After(2 * time.Second):
								fmt.Println(err)
							}
						}
						if newState == currentState.State && *cm.Metadata.Name == currentState.Name {
							continue
						}
						update := K8sStateUpdate{
							Name:  *cm.Metadata.Name,
							State: newState,
						}
						// Try sending the new state to listeners within the server invoking this function
						select {
						case updateC <- update:
							currentState = update
						case <-time.After(2 * time.Second):
							// If the message could not be sent try to wakeup the error logger
							msg := kv.NewError("could not update state").With("namespace", namespace).With("config", *cm.Metadata.Name).With("state", state).With("stack", stack.Trace().TrimRuntime())
							select {
							case errC <- msg:
							case <-time.After(2 * time.Second):
								fmt.Println(msg, stack.Trace().TrimRuntime())
							}
							continue
						}
					}
				}
			}
		}
	}(ctx)

	return nil
}
