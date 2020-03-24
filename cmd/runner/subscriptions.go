// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"sync"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
)

// This file contains the implementation of a collection of subscriptions, typically
// references to queues that are maintained within a project, typically a queue server.

// Subscription is used to encapsulate the details of a single queue subscription including the resources
// that subscription has requested for its work in the past and how many instances of work units
// are currently being processed by this server
//
type Subscription struct {
	name string           // The subscription name that represents a queue of potential for our purposes
	rsc  *runner.Resource // If known the resources that experiments asked for in this subscription
	cnt  uint             // The number of instances that are running for this queue
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

	subs.Lock()
	defer subs.Unlock()

	for sub := range expected {
		if _, isPresent := subs.subs[sub]; !isPresent {

			subs.subs[sub] = &Subscription{name: sub}
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
func (subs *Subscriptions) setResources(name string, rsc *runner.Resource) (err kv.Error) {
	if rsc == nil {
		return kv.NewError("clearing the resource spec for the subscription "+name+" is not supported").With("stack", stack.Trace().TrimRuntime())
	}

	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError(name+" was not present").With("stack", stack.Trace().TrimRuntime())
	}

	q.rsc = rsc

	return nil
}

func (subs *Subscriptions) incWorkers(name string) (err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError(name+" was not present").With("stack", stack.Trace().TrimRuntime())
	}

	q.cnt++
	return nil
}

func (subs *Subscriptions) decWorkers(name string) (err kv.Error) {
	subs.Lock()
	defer subs.Unlock()

	q, isPresent := subs.subs[name]
	if !isPresent {
		return kv.NewError(name+" was not present").With("stack", stack.Trace().TrimRuntime())
	}

	q.cnt--
	return nil
}
