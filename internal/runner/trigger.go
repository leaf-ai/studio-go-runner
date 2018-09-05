package runner

// This file implements a timer and a trigger channel together into a single
// channel, this is useful for when we wish an action to be triggered on
// a regular basis and also want to allow a caller to manually invoke, or
// trigger and action.  This is often used in testing.

import (
	"time"

	"github.com/lthibault/jitterbug"
)

type Trigger struct {
	quitC chan struct{}
	tick  *jitterbug.Ticker
	T     <-chan struct{}
	C     chan time.Time
}

func NewTrigger(triggerC <-chan struct{}, d time.Duration, j jitterbug.Jitter) (t *Trigger) {
	t = &Trigger{
		tick:  jitterbug.New(d, j),
		T:     triggerC,
		C:     make(chan time.Time, 1),
		quitC: make(chan struct{}, 1),
	}
	go t.loop()
	return t
}

func (t *Trigger) Stop() {
	t.Stop()
}

func (t *Trigger) loop() {
	defer func() {
		close(t.C)
	}()

	for {
		select {
		case <-t.quitC:
			return
		case <-t.tick.C:
			t.signal()
		case <-t.T:
			t.signal()
		}
	}
}

func (t *Trigger) signal() {
	select {
	case t.C <- time.Now():
	case <-time.After(200 * time.Millisecond):
	}
}
