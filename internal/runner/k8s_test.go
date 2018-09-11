package runner

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/types"

	"github.com/ericchiang/k8s"

	core "github.com/ericchiang/k8s/apis/core/v1"
	meta "github.com/ericchiang/k8s/apis/meta/v1"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/rs/xid"
)

func createCMap(ctx context.Context, namespace string, name string, k string, v string) (err errors.Error) {
	configMap := &core.ConfigMap{
		Metadata: &meta.ObjectMeta{
			Name:      k8s.String(name),
			Namespace: k8s.String(namespace),
		},
		Data: map[string]string{k: v},
	}

	client, errGo := k8s.NewInClusterClient()
	if errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if errGo = client.Create(ctx, configMap); errGo != nil {
		return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return nil
}

var (
	values = struct {
		data   map[string]map[string]string
		rwLock sync.RWMutex
	}{
		data: make(map[string]map[string]string),
	}
)

// This file contains a number of tests that if Kubernetes is detected as the runtime
// the test is being hosted in will be activated and used

func TestK8sConfig(t *testing.T) {
	logger := NewLogger("k8s_configmap_test")
	if err := IsAliveK8s(); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Establish a listener for the API under test
	updateC := make(chan K8sStateUpdate, 1)
	errC := make(chan errors.Error, 1)

	namespace := "default"
	name := "test_" + xid.New().String()

	if err := ListenK8s(ctx, namespace, name, "", updateC, errC); err != nil {
		t.Fatal(err)
	}

	// Go and create a k8s config map that we can use for testing purposes
	if err := createCMap(ctx, namespace, name, "STATE", "Running"); err != nil {
		t.Fatal(err)
	}

	// Now see if we get the state change with "Running"
	func() {
		for {
			select {
			case <-ctx.Done():
				t.Fatal(errors.New("timeout waiting for k8s configmap to change state").With("stack", stack.Trace().TrimRuntime()))
			case state := <-updateC:
				if state.Name == name && state.State == types.K8sRunning {
					return
				}
			}
		}
	}()

	// Register a listener for the newly created map
	// Change the map and see if things get notified

	logger.Info("TestK8sConfig completed")
}
