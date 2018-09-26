// +build !NO_CUDA

package runner

import (
	"testing"
)

// This file contains an integration test implementation that submits a studio runner
// task across an SQS queue and then validates is has completed successfully by
// the go runner this test is running within

func TestCUDAActive(t *testing.T) {
	logger := NewLogger("cuda_active_test")
	if !*UseGPU {
		logger.Warn("TestCUDA not run")
		t.Skip("no GPUs present for testing")
	}

	devs, err := getCUDAInfo()
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) < 1 {
		t.Fatal("no CUDA capable devices found during the CUDA testing")
	}

	logger.Warn("cuda_active_test completed")
}
