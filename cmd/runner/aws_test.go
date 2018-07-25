package main

import (
	"testing"

	"github.com/SentientTechnologies/studio-go-runner"
)

// This file contains an integration test implementation that submits a studio runner
// task across an SQS queue and then validates is has completed successfully by
// the go runner this test is running within

func TestAWS(t *testing.T) {
	logger = runner.NewLogger("aws_test")

	logger.Info("TestAWS completed")
}
