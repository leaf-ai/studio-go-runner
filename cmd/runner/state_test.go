package main

import (
	"context"
	"flag"
	"strings"
	"testing"
	"time"

	"github.com/rs/xid"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	useK8s = flag.Bool("use-k8s", false, "Enables any Kubernetes cluster specific tests")
)

// This file contains the implementation of a test that will simulate a state change
// for the server and will verify that the schedulers respond appropriately. States
// are controlled using kubernetes and so this test will exercise the state management
// without using the k8s modules, these are tested seperately

// TestBroadcast tests the fan-out of Kubernetes state updates using Go channels. This is
// primarily a unit test when for the k8s cluster is not present
//
func TestBroadcast(t *testing.T) {

	// Use runner.NewStateBroadcast to create a master channel
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(5*time.Second))
	defer cancel()

	errorC := make(chan errors.Error, 1)

	l := runner.NewStateBroadcast(ctx, errorC)

	// Create three listeners
	listeners := []chan runner.K8sStateUpdate{
		make(chan runner.K8sStateUpdate, 1),
		make(chan runner.K8sStateUpdate, 1),
		make(chan runner.K8sStateUpdate, 1),
	}
	for _, listener := range listeners {
		l.Add(listener)
	}

	failed := false
	err := errors.New("")
	doneC := make(chan struct{}, 1)

	go func() {
		defer close(doneC)
		// go routine the listeners, with early finish if they are receive traffic
		for _, listener := range listeners {
			select {
			case <-listener:
			case <-ctx.Done():
				err = errors.New("one of the listeners received no first message").With("stack", stack.Trace().TrimRuntime())
				failed = true
				return
			}
		}
		// Now check that no listener gets a second message
		for _, listener := range listeners {
			select {
			case <-listener:
				err = errors.New("one of the listeners received an unexpected second message").With("stack", stack.Trace().TrimRuntime())
				failed = true
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()

	// send something out, let it be consumed and if it is not then we have an issue
	select {
	case l.Master <- runner.K8sStateUpdate{
		State: types.K8sRunning,
		Name:  xid.New().String(),
	}:
	case <-ctx.Done():
		t.Fatal(errors.New("the master channel could not be used to send a broadcast").With("stack", stack.Trace().TrimRuntime()))
	}

	// Now wait for the receiver to do its thing
	select {
	case <-doneC:
	case <-ctx.Done():
		t.Fatal(errors.New("the receiver channel(s) timed out").With("stack", stack.Trace().TrimRuntime()))
	}

	// see what happened
	if failed {
		t.Fatal(err)
	}
}

// TestStates will exercise the internal changing of states within the queue processing
// of the server.  It tests the state changes without using the kubernetes side.  The k8s
// testing is done in a specific test case that just tests that component when the
// test is run within a working cluster.  To do this properly k8s should be used with a
// bundled rabbitMQ server.
//
func TestStates(t *testing.T) {

	if !*useK8s {
		t.Skip("kubernetes specific testing disabled")
	}

	// We need a queuing system up and running because the states and queue states that
	// are tracked in prometheus will only update in our production code when the
	// scheduler actually finds a reference to some queuing
	if err := runner.PingRMQServer(*amqpURL); err != nil {
		t.Fatal(err)
	}

	// send bogus updates by instrumenting the lifecycle listeners in cmd/runner/k8s.go
	select {
	case k8sStateUpdates().Master <- runner.K8sStateUpdate{State: types.K8sRunning}:
	case <-time.After(time.Second):
		t.Fatal("state change could not be sent, no master was listening")
	}

	pClient := NewPrometheusClient("http://localhost:9090/metrics")

	foundRefreshers := false
	timeout := time.NewTicker(time.Minute)
	timeout.Stop()

	for !foundRefreshers {
		select {
		case <-timeout.C:
			t.Fatal()
		case <-time.After(2 * time.Second):
			metrics, err := pClient.Fetch("runner_queue_")
			if err != nil {
				t.Fatal(err)
			}
			for k, _ := range metrics {
				if strings.Contains(k, "runner_queue_checked") {
					foundRefreshers = true
				}
			}
		}
	}
	timeout.Stop()

	// send bogus updates by instrumenting the lifecycle listeners in cmd/runner/k8s.go
	select {
	case k8sStateUpdates().Master <- runner.K8sStateUpdate{State: types.K8sDrainAndSuspend}:
	case <-time.After(time.Second):
		t.Fatal("state change could not be sent, no master was listening")
	}

	// Retrieve prometheus counters to aws, google, and rabbit queue implementations

	defer func() {
		logger.Info("server state returning to running")

		select {
		case k8sStateUpdates().Master <- runner.K8sStateUpdate{State: types.K8sRunning}:
		case <-time.After(time.Second):
			logger.Warn("state reset could not be sent, no master was listening")
		}
	}()

	timer := time.NewTicker(time.Second)

	// see what the prometheus counters do and make sure they match our drained state
	func() {
		defer timer.Stop()

		ignoredChanged := time.Now() // Tracks when the ignored metric changed
		ignored := 0                 // This counts all of the ignored queues
		ignoredSinceLastChecked := 0 // This counts the times the ignored counter was bump since the last check counter change

		checkedChanged := time.Now() // Tracks when the checked metric changed
		checked := 0                 // This count the last known number of checked queues

		for {
			select {
			case <-timer.C:
				metrics, err := pClient.Fetch("runner_queue_")
				if err != nil {
					t.Fatal(err)
				}
				for k, metric := range metrics {
					switch k {
					case "runner_queue_ignored":
						// Track the number of ignored queue checks.  We want this
						// to increase in this case for a time without the
						// successful checks increasing.  This will validate that
						// the drained state is being respected by the server
						total := 0
						for _, m := range metric.GetMetric() {
							total += int(*m.GetCounter().Value)
						}
						// If we have yet to get any ignored tracking we initialize it
						if ignored == 0 {
							ignoredChanged = time.Now()
							ignored = total
							continue
						}
						// Track the number of times that the ignored count is stable
						if ignored != total {
							ignoredChanged = time.Now()
							ignored = total
							ignoredSinceLastChecked++
							continue
						}

					case "runner_queue_checked":
						total := 0
						for _, m := range metric.GetMetric() {
							total += int(*m.GetCounter().Value)
						}
						// If we have yet to get any checked tracking we initialize it
						if checked == 0 {
							checkedChanged = time.Now()
							checked = total
							continue
						}
						// Track the number of times that the checked count is stable
						if checked != total {
							ignoredSinceLastChecked = 0
							checkedChanged = time.Now()
							checked = total
							continue
						}
					}
				} // End of for k, v := range metrics
			}
			// The checked counters should not have changed after the ignored counters were,
			// if so the server has not yet respected the change in state
			if ignoredChanged.Before(checkedChanged) {
				continue
			}

			// If the ignored counter was modified at least twice since the last
			// checked total changed then we assume the server has respected the change
			if ignoredSinceLastChecked >= 2 {
				return
			}
		}
	}()

	// Consider someway to combining some elements of the three of them
	// Consider splitting out the lifecycle listeners channel side into a channel pattern library
	// done
}
