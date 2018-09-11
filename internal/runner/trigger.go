package runner

// This file implements a timer and a trigger channel together into a single
// channel, this is useful for when we wish an action to be triggered on
// a regular basis and also want to allow a caller to manually invoke, or
// trigger and action.  This is often used in testing.

import (
	"time"

	"github.com/lthibault/jitterbug"
)

// Trigger is a data structure that encapsulates a timer and a channel which together are used to in turn
// to send messages to a downstream go channel.  The main Trigger use case is to allow a regular action
// to be scheduled via a timer and also to allow unit tests for example to activate the same action.
//
type Trigger struct {
	quitC chan struct{}
	tick  *jitterbug.Ticker
	T     <-chan struct{}
	C     chan time.Time
}

// NewTrigger accepts a timer and a channel that together can be used to send messages
// into a channel that is encapsulated within the returned t data structure
//
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

// Stop will close the internal channel used to signal termination to the
// internally running go routine that processes the timer and the trigger
// chanel
//
func (t *Trigger) Stop() {
	close(t.quitC)
}

// loop is the internal service go routine that will accept either a timer,
// or the manually notified channel to trigger the downstream channel.
//
// loop also listens for termnination and will tear everything down if
// that occurs
//
func (t *Trigger) loop() {
	defer func() {
		t.tick.Stop()

		close(t.C)

		// Typically the termination will be seen as a nil
		// message on the channel which is the close occuring
		// elsewhere.  Close again for safety sake but
		// ignore a panic if the channel is already down
		defer func() {
			recover()
		}()

		close(t.quitC)
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

// signal is called to trigger the downstream channel with a timeout if
// no one is listening in order that it does not block
//
func (t *Trigger) signal() {
	select {
	case t.C <- time.Now():
	case <-time.After(200 * time.Millisecond):
	}
}
