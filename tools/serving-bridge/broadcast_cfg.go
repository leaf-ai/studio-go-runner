// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/jjeffery/kv" // MIT License
)

// This file contains the implementation of a channel fan-out
// based on subscriptions for the config data structure.
//

type ConfigOptionals struct {
	endpoint  *string
	accessKey *string
	secretKey *string
	bucket    *string
}

// Listeners is used to handle the broadcasting of cluster events when Kubernetes is
// being used
type Listeners struct {
	SendingC   chan ConfigOptionals
	listeners  map[xid.ID]chan<- Config
	currentCfg Config
	sync.Mutex
}

// NewConfigBroadcast is used to instantiate a configuration update broadcaster
func NewConfigBroadcast(ctx context.Context, cfg Config, errorC chan<- kv.Error) (l *Listeners) {
	l = &Listeners{
		SendingC:   make(chan ConfigOptionals, 1),
		listeners:  map[xid.ID]chan<- Config{},
		currentCfg: cfg,
	}

	go l.run(ctx, errorC)

	return l
}

func (l *Listeners) run(ctx context.Context, errorC chan<- kv.Error) {
	for {
		select {
		case <-ctx.Done():
			return
		case cfg := <-l.SendingC:

			// Only some fields maybe intended for update operations so check their length and
			// only incorporate filled out fields
			l.Lock()
			if cfg.endpoint != nil {
				l.currentCfg.endpoint = *cfg.endpoint
			}
			if cfg.accessKey != nil {
				l.currentCfg.accessKey = *cfg.accessKey
			}
			if cfg.secretKey != nil {
				l.currentCfg.secretKey = *cfg.secretKey
			}
			if cfg.bucket != nil {
				l.currentCfg.bucket = *cfg.bucket
			}
			l.Unlock()

			clients := make([]chan<- Config, 0, len(l.listeners))

			// Make a consistent copy of all the channels that the update will be sent down
			// so that we retain the values at this moment in time
			if len(l.listeners) != 0 {
				l.Lock()
				for _, v := range l.listeners {
					clients = append(clients, v)
				}
				l.Unlock()
			}

			for _, c := range clients {
				func() {
					defer func() {
						// There is a window of time in which the delete for a listener occurs
						// between copying the collection of listeners and someone else
						// deleting the listen and this function then doing a send
						recover()
					}()
					select {
					case c <- l.currentCfg:
					case <-time.After(500 * time.Millisecond):
					}
				}()
			}
		}
	}
}

// Add is used when a running thread wishes to add a channel to the broadcaster
// on which config change events will be received
func (l *Listeners) Add(listen chan<- Config) (id xid.ID, err kv.Error) {

	id = xid.New()

	l.Lock()
	l.listeners[id] = listen
	initialCfg := l.currentCfg
	l.Unlock()

	// Send an initial authoritive copy of the configuration down the channel
	go func() {
		listen <- initialCfg
	}()

	return id, nil
}

// Delete is used when a running thread wishes to drop a channel from the broadcaster
// on which config events will be received
func (l *Listeners) Delete(id xid.ID) {

	l.Lock()
	delete(l.listeners, id)
	l.Unlock()
}
