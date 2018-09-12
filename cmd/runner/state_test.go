package main

import (
	"context"
	"testing"
	"time"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/rs/xid"
)

// This file contains the implementation of a test that will simulate a state change
// for the server and will verify that the schedulers respond appropriately. States
// are controlled using kubernetes and so this test will exercise the state management
// without using the k8s modules, these are tested seperately

// TestStates will excercise the internal changing of states within the queue processing
// of the server.  It tests the state changes without using the kubernetes side.  The k8s
// testing is done in a specific test case that just tests that component when the
// test is run within a working cluster.
//
func TestStates(t *testing.T) {
	logger := runner.NewLogger("test_states")

	// Add prometheus counters to aws, google, and rabbit queue implementations
	// Consider someway to combining some elements of the three of them
	// send bogus updates by instrumenting the lifecycle listeners in c/r/k8s.go
	// Consider splitting out the lifecycle listeners channel side into a channel pattern library
	// Send the biogus signal
	// see what the prometheus counters do
	// done

	logger.Info("test_states done")
}

// TestBroadcast tests the fan-out of Kubernetes state updates using Go channels. This is
// primarily a unit test when for the k8s cluster is not present
//
func TestBroadcast(t *testing.T) {
	logger := runner.NewLogger("test_broadcast")
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

	logger.Info("test_broadcast done")
}
