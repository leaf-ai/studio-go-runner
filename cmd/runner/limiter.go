// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/go-stack/stack"
	uberatomic "go.uber.org/atomic"
)

// This contains the implementation of a task limiter which will be used
// to control the task acceptance from queues.

var (
	maxTasksOpt      = flag.Uint("limit-tasks", 0, "maximum number of tasks after which the runner will drain and termiate (default 0, never terminate due to task completed counts)")
	maxIdleOpt       = flag.Duration("limit-idle-duration", time.Duration(0), "maximum idle timer after which the runner will drain and terminate (default 0s, never terminate)")
	limitIntervalOpt = flag.Duration("limit-interval", time.Duration(5*time.Minute), "timer for checking for termination (default 0s, minimum 5 minutes, dont terminate due to idling)")

	noNewTasks = uberatomic.NewBool(false)

	LimitCheck = time.Duration(0)
)

const (
	minimumLimitInterval = time.Duration(5 * time.Minute)
)

type activity struct {
	idle time.Time // Time when the last running count of zero was observed
}

func limitCheck(acts *activity) (limit bool, msg string) {
	if noNewTasks.Load() {
		return true, ""
	}

	if *maxTasksOpt == 0 && *maxIdleOpt == time.Duration(0) {
		msg = fmt.Sprint("task limits not in use", "stack", stack.Trace().TrimRuntime())
		return false, msg
	}
	running, err := GetGaugeAccum(queueRunning)
	if err != nil {
		msg = fmt.Sprint("error", err.Error(), "stack", stack.Trace().TrimRuntime())
		return false, msg
	}
	if !almostEqual(running, 0.0) {
		acts.idle = time.Now().Add(*maxIdleOpt)
		logger.Debug("idle time reset", "stack", stack.Trace().TrimRuntime())
	}
	// Now see how many tasks have run in the system
	ran, err := GetCounterAccum(queueRan)
	if err != nil {
		msg = fmt.Sprint("error", err.Error(), "stack", stack.Trace().TrimRuntime())
		return false, msg
	}

	logger.Debug("ready to check counted limit", "stack", stack.Trace().TrimRuntime())

	// See if the total of running tasks and ran tasks equals or exceed the maximum number
	// this runner has been configured to handle
	if *maxTasksOpt != 0 && math.Round(running+ran) > math.Round(float64(*maxTasksOpt)) {
		// See if we are drained and the max run count has been reached
		if !almostEqual(running, 0.0) {
			msg = fmt.Sprint("stack", stack.Trace().TrimRuntime())
			// Conditions are correct for us to shutdown, we are idle and enough tasks have been done,
			// but a task is still active
			return true, msg
		}
		noNewTasks.Store(true)
		return true, msg
	}

	logger.Debug("ready to check idle limit", "running", running, "idle", acts.idle, "max idle time", *maxIdleOpt, "stack", stack.Trace().TrimRuntime())

	// If nothing is running and the last time we saw anything running was more than the idle timer
	// then we can stop, as long as the user specified a maximum idle time that was not zero
	if almostEqual(running, 0.0) && acts.idle.Before(time.Now()) && *maxIdleOpt != time.Duration(0) {
		msg = fmt.Sprint("stack", stack.Trace().TrimRuntime())
		return true, msg
	}
	return false, ""
}

// serviceLimiter is used to monitor the runner counters and idle timers and if appropriate respond to
// any limiting conditions that are meet, for example idle timers, and respond by shutting the service down
// for example
func serviceLimiter(ctx context.Context, cancel context.CancelFunc) {

	defer func() {
		_ = recover()
		cancel()
	}()

	checkInterval := *limitIntervalOpt
	if checkInterval < minimumLimitInterval {
		checkInterval = minimumLimitInterval
	}
	LimitCheck = checkInterval

	check := time.NewTicker(*limitIntervalOpt)
	defer check.Stop()

	acts := activity{
		idle: time.Now().Add(*maxIdleOpt),
	}

	// Suppress duplicate logs
	lastMsg := ""
	lastRepeatedAfter := time.Duration(15 * time.Minute)
	lastPrinted := time.Unix(0, 0)

	for {
		select {
		case <-check.C:
			limited, msg := limitCheck(&acts)
			if msg != lastMsg || lastPrinted.Before(time.Now()) {
				lastMsg = msg
				lastPrinted = time.Now().Add(lastRepeatedAfter)
				logger.Warn(msg)
			}
			if limited {
				logger.Info("limiter was triggered", "stack", stack.Trace().TrimRuntime())
				return
			}

		case <-ctx.Done():
			return
		}
	}
}
