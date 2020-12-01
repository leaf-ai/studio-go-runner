// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package components

// This file contains the implementation of a map of strings and booleans that are updated
// by a servers critical components.  Clients can also request that if any of the components goes into
// a down (false) condition that they will be informed using a channel and when the entire collection
// goes into a true condition that they will also be informed.
//
// This allows for a server to maintain an overall check on whether any of its critical
// components are down and when they are all alive which is useful in health checking.

import (
	"sync"
	"time"

	"golang.org/x/net/context"
)

type Components struct {
	listeners    []chan bool
	components   map[string]bool
	clientUpdate chan struct{}
	sync.Mutex
}

func (comps *Components) AddListener(listener chan bool) {
	comps.Lock()
	defer comps.Unlock()
	comps.listeners = append(comps.listeners, listener)
}

func (comps *Components) SetModule(module string, up bool) {
	comps.Lock()
	defer comps.Unlock()
	comps.components[module] = up
	select {
	case comps.clientUpdate <- struct{}{}:
	default:
	}
}

func (comps *Components) doUpdate() {
	comps.Lock()
	defer comps.Unlock()

	downModules := make([]string, 0, len(comps.components))
	upModules := make([]string, 0, len(comps.components))

	// Is the sever entirely up or not
	up := true
	for k, v := range comps.components {
		if v != true {
			up = false
			downModules = append(downModules, k)
		} else {
			upModules = append(upModules, k)
		}
	}

	// Tell everyone what the collective state is for the server
	for i, listener := range comps.listeners {
		func() {
			defer func() {
				// A send to a closed channel will panic and so if a
				// panic does occur we remove the listener
				if r := recover(); r != nil {
					comps.Lock()
					defer comps.Unlock()

					if len(comps.listeners) <= 1 {
						comps.listeners = []chan bool{}
						return
					}
					comps.listeners = append(comps.listeners[:i], comps.listeners[i+1:]...)
				}
			}()
			select {
			case <-time.After(20 * time.Millisecond):
			case listener <- up:
			}
		}()
	}
}

func InitComponentTracking(ctx context.Context) (comps *Components) {
	comps = &Components{
		listeners:    []chan bool{},
		components:   map[string]bool{},
		clientUpdate: make(chan struct{}),
	}

	go func(comps *Components) {
		internalCheck := time.Duration(5 * time.Second)
		for {
			select {

			case <-time.After(internalCheck):
				comps.doUpdate()
			case <-comps.clientUpdate:
				comps.doUpdate()
			case <-ctx.Done():
				return
			}
		}
	}(comps)

	return comps
}
