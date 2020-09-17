// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package studio

import (
	"context"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/jjeffery/kv" // MIT License
)

// This file contains the implementation of a channel fan-out
// based on subscriptions.
//

// Listeners is used to handle the broadcasting of cluster events when Kubernetes is
// being used
type Listeners struct {
	Master    chan K8sStateUpdate
	listeners map[xid.ID]chan<- K8sStateUpdate
	sync.Mutex
}

// NewStateBroadcast is used to instantiate a Kubernetes event broadcaster
func NewStateBroadcast(ctx context.Context, errorC chan<- kv.Error) (l *Listeners) {
	l = &Listeners{
		Master:    make(chan K8sStateUpdate, 1),
		listeners: map[xid.ID]chan<- K8sStateUpdate{},
	}

	go l.run(ctx, errorC)

	return l
}

func (l *Listeners) run(ctx context.Context, errorC chan<- kv.Error) {
	for {
		select {
		case <-ctx.Done():
			return
		case state := <-l.Master:

			clients := make([]chan<- K8sStateUpdate, 0, len(l.listeners))

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
					case c <- state:
					case <-time.After(500 * time.Millisecond):
					}
				}()
			}
		}
	}
}

// Add is used when a running thread wishes to add a channel to the broadcaster
// on which Kubernetes events will be received
func (l *Listeners) Add(listen chan<- K8sStateUpdate) (id xid.ID, err kv.Error) {

	id = xid.New()

	l.Lock()
	l.listeners[id] = listen
	l.Unlock()

	return id, nil
}

// Delete is used when a running thread wishes to drop a channel from the broadcaster
// on which Kubernetes events will be received
func (l *Listeners) Delete(id xid.ID) {

	l.Lock()
	delete(l.listeners, id)
	l.Unlock()
}
