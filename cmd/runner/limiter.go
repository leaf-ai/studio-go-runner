// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	uberatomic "go.uber.org/atomic"
)

// This contains the implementation of a task limiter which will be used
// to control the task acceptance from queues.

var (
	maxTasksOpt      = flag.Uint("max-tasks", 0, "maximum number of tasks after which the runner will drain and termiate (default 0, never terminate due to task completed counts)")
	maxIdleOpt       = flag.Duration("max-idle-duration", time.Duration(0), "maximum idle timer after which the runner will drain and terminate (default 0s, never terminate)")
	limitIntervalOpt = flag.Duration("limit-interval", time.Duration(5*time.Minute), "timer for checking for termination (default 0s, minimum 5 minutes, dont terminate due to idling)")

	noNewTasks = uberatomic.NewBool(false)
)

const (
	minimumLimitInterval = time.Duration(5 * time.Minute)
)

type activity struct {
	idle    time.Time // Time when the last running count of zero was observed
	running float64   // Prometheus count of running tasks
	ran     float64   // Prometheus count of tasks that have completed
}

func serviceLimiter(ctx context.Context, cancel context.CancelFunc) {

	// If there are no limits to the lifetime of the server
	// dont setup any limit checking
	if *maxTasksOpt == 0 && *maxIdleOpt == time.Duration(0) {
		logger.Info("runner task limits not in use")
		return
	}

	checkInterval := *limitIntervalOpt
	if checkInterval < minimumLimitInterval {
		checkInterval = minimumLimitInterval
	}

	check := time.NewTicker(*limitIntervalOpt)
	defer check.Stop()

	acts := activity{
		idle: time.Now(),
	}
	err := kv.NewError("")

	// Suppress duplicate logs
	lastMsg := ""
	lastRepeatedAfter := time.Duration(15 * time.Minute)
	lastPrinted := time.Now().Add(lastRepeatedAfter)

	for {
		select {
		case <-check.C:
			// Get the current running count and if it is non zero then we update the
			// idle time to represent now so that the maximum idle timer is not activated
			if acts.running, err = GetGaugeAccum(queueRunning); err != nil {
				msg := fmt.Sprint("error", err.Error(), "stack", stack.Trace().TrimRuntime())
				if msg != lastMsg || lastPrinted.Before(time.Now()) {
					lastMsg = msg
					lastPrinted = time.Now().Add(lastRepeatedAfter)
					logger.Warn(msg)
				}
				continue
			}
			if !almostEqual(acts.running, 0.0) {
				acts.idle = time.Now().Add(*maxIdleOpt)
			}
			// Now see how many tasks have run in the system
			if acts.ran, err = GetCounterAccum(queueRan); err != nil {
				msg := fmt.Sprint("error", err.Error(), "stack", stack.Trace().TrimRuntime())
				if msg != lastMsg || lastPrinted.Before(time.Now()) {
					lastMsg = msg
					lastPrinted = time.Now().Add(lastRepeatedAfter)
					logger.Warn(msg)
				}
				continue
			}

			// See if the total of running tasks and ran tasks equals or exceed the maximum number
			// this runner has been configured to handle
			if *maxTasksOpt != 0 && math.Round(acts.running+acts.ran) > math.Round(float64(*maxTasksOpt)) {
				// See if we are drained and the max run count has been reached
				if !almostEqual(acts.running, 0.0) {
					msg := fmt.Sprint("stack", stack.Trace().TrimRuntime())
					if msg != lastMsg || lastPrinted.Before(time.Now()) {
						lastMsg = msg
						lastPrinted = time.Now().Add(lastRepeatedAfter)
						logger.Warn("max run tasks reached, signalling system stop", msg)
					}
					// Conditions are correct for us to shutdown, we are idle and enough tasks have been done
					defer func() {
						_ = recover()
					}()
					cancel()
					return
				}
				noNewTasks.Store(true)
			}

			// If nothing is running and the last time we saw anything running was more than the idle timer
			// then we can stop, as long as the user specified a maximum idle time that was not zero
			if almostEqual(acts.running, 0.0) && acts.idle.Before(time.Now()) && *maxIdleOpt != time.Duration(0) {
				msg := fmt.Sprint("stack", stack.Trace().TrimRuntime())
				if msg != lastMsg || lastPrinted.Before(time.Now()) {
					lastMsg = msg
					lastPrinted = time.Now().Add(lastRepeatedAfter)
					logger.Warn("max idle time reached, signalling system stop", msg)
				}
				defer func() {
					_ = recover()
				}()
				cancel()
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
