package runner

import (
	"context"
	"sync"
	"time"

	"github.com/rs/xid"

	"github.com/karlmutch/errors"
)

// This file contains the implementation of a channel fan-out
// based on subscriptions.
//

type Listeners struct {
	Master    chan K8sStateUpdate
	listeners map[xid.ID]chan<- K8sStateUpdate
	sync.Mutex
}

func NewStateBroadcast(ctx context.Context, errorC chan<- errors.Error) (l *Listeners) {
	l = &Listeners{
		Master:    make(chan K8sStateUpdate, 1),
		listeners: map[xid.ID]chan<- K8sStateUpdate{},
	}

	go l.run(ctx, errorC)

	return l
}

func (l *Listeners) run(ctx context.Context, errorC chan<- errors.Error) {
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
				select {
				case c <- state:
				case <-time.After(500 * time.Millisecond):
				}
			}
		}
	}
}

func (l *Listeners) Add(listen chan<- K8sStateUpdate) (id xid.ID, err errors.Error) {

	id = xid.New()
	l.Lock()
	defer l.Unlock()

	l.listeners[id] = listen

	return id, nil
}

func (l *Listeners) Delete(id xid.ID) {

	l.Lock()
	defer l.Unlock()

	delete(l.listeners, id)
}
