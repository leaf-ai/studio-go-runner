// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.
package main

import (
	"fmt"
	"testing"
	"time"
)

// This file contains test code for tests that allow the user to pause in
// testing mode until the idle timer fires or another limit has been reached and
// then to observe the runners internal serving code terminate as a form
// of shutdown testing

// TestĆLimiterShutdown is the test case version of the ValidateĆLimiterShutdown test logic
// that can be scheduled by the Go test sub system
func TestĆLimiterShutdown(t *testing.T) {

	// Examine the idle time and if set we would spin out the validate until we knew the idle had kicked in
	ValidateĆLimiterShutdown(t)
}

// ValidateĆLimiterShutdown is used to ensure that the correct limiter behavior is
// occuring within the test server at the termination of any other tests using
// the user specific limiter options that can be scraped from the configuration
// options.
//
// The Validate function is intended to be run by any end-to-end tests that determine
// the server should have responded in someway to an idle timer or tasks completed count
// by shutting down the server.
//
func ValidateĆLimiterShutdown(t *testing.T) {
	if *maxIdleOpt == time.Duration(0) && *maxTasksOpt == 0 {
		t.Skip("shutdown testing not applicable")
	}

	timeout := *maxIdleOpt + *limitIntervalOpt + time.Second
	// As this is the last test ever run we can obtain the count of running tasks which should be zero
	// along with the idle timeout option which should give us a predictable time for the servers termination
	running, err := GetGaugeAccum(queueRunning)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !almostEqual(running, 0.0) {
		t.Fatal(fmt.Sprint(running, " tasks are still running"))
	}
	select {
	case <-time.After(timeout):
		t.Fatal("shutdown was not signalled")
		// Get the main server context that can be used to determine if the server has
		// signalled its own shutdown
	case <-serverShutdown.Done():
		return
	}
}
