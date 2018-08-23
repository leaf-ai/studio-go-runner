package runner

import (
	"fmt"
	"testing"
)

// This file contains a number of tests that if Kubernetes is detected as the runtime
// the test is being hosted in will be activated and used

func TestK8sConfig(t *testing.T) {
	logger := NewLogger("k8s_configmap_test")
	if err := IsAliveK8s(); err != nil {
		logger.Warn(fmt.Sprint(err))
	}
	logger.Info("TestK8sConfig completed")
}
