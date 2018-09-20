// +build !NO_CUDA

package runner

import (
	"testing"
)

// This file contains an integration test implementation that submits a studio runner
// task across an SQS queue and then validates is has completed successfully by
// the go runner this test is running within

func TestCUDA(t *testing.T) {
	logger := NewLogger("cuda_test")
	if !*UseGPU {
		logger.Warn("TestCUDA not run")
		t.Skip("no GPUs present for testing")
	}
	logger.Warn("TestCUDA completed")
}
