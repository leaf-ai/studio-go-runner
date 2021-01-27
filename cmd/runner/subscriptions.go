// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
)

var (
	execEMAwindow = time.Duration(time.Hour)
)

// This file contains the implementation of a collection of subscriptions, typically
// references to queues that are maintained within a project, typically a queue server.

// Subscription is used to encapsulate the details of a single queue subscription including the resources
// that subscription has requested for its work in the past and how many instances of work units
// are currently being processed by this server
//
type Subscription struct {
	name     string           // The subscription name that represents a queue of potential for our purposes
	rsc      *server.Resource // If known the resources that experiments asked for in this subscription
	inFlight uint             // The number of instances that are running for this queue
	execAvgs *runner.TimeEMA  // A set of exponential moving averages for execution times
}

// Subscriptions stores the known activate queues/subscriptions that this runner has observed
//
type Subscriptions struct {
	subs map[string]*Subscription // The catalog of all known queues (subscriptions) within the project this server is handling
	sync.Mutex
}

// align allows the caller to take the extant subscriptions and add or remove them from the list of subscriptions
// we currently have cached
//
func (subs *Subscriptions) align(expected map[string]interface{}) (added []string, removed []string) {

	added = []string{}
	removed = []string{}

	execEMAWindows := []time.Duration{time.Duration(10 * time.Minute), execEMAwindow}
	subs.Lock()
	defer subs.Unlock()

	for sub := range expected {
		if _, isPresent := subs.subs[sub]; !isPresent {

			// Save an exponential moving average for the runtimes
			subs.subs[sub] = &Subscription{
				name:     sub,
				execAvgs: runner.NewTimeEMA(execEMAWindows, time.Duration(30*time.Minute)),
			}
			added = append(added, sub)
		}
	}

	for sub := range subs.subs {
		if _, isPresent := expected[sub]; !isPresent {

			delete(subs.subs, sub)
			removed = append(removed, sub)
		}
	}

	return added, removed
}

// setResources is used to update the resources a queue will generally need for
// its individual work items
//
func (subs *Subscriptions) setResources(name string, rsc *server.Resource) (err kv.Error) {
	if rsc == nil {
		return kv.NewError("clearing the resource spec not supported").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError("subscription not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	q.rsc = rsc

	return nil
}

func (subs *Subscriptions) incWorkers(name string) (err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError("subscription not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	q.inFlight++
	return nil
}

func (subs *Subscriptions) decWorkers(name string) (err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError("subscription not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	q.inFlight--
	return nil
}

func (subs *Subscriptions) execTime(name string, execTime time.Duration) (err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError("subscription not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	q.execAvgs.Update(execTime)
	return nil
}

func (subs *Subscriptions) getExecAvg(name string) (execTime time.Duration, err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return time.Duration(0), kv.NewError("subscription not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	exec, isPresent := q.execAvgs.Get(execEMAwindow)
	if !isPresent {
		return time.Duration(0), kv.NewError("execution EMA not found").With("subscription", name).With("stack", stack.Trace().TrimRuntime())

	}
	return exec, nil
}
