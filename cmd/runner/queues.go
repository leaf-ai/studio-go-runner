// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of queue query functions along with
// tracking for queues to detect new arrivals and also to detect the
// disappearance of queues
//
// As queues come and go subscriptions are automatically created/accessed so that
// messages have a chance to be noticed

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus"
)

var (
	// backoffs are a set of subscriptions to queues that when they are still alive
	// in the cache the server will not attempt to retrieve work from.  When the
	// cache entries that represent the subscriptions expire then they are
	// deemed to be ready for retrieving more work from.
	//
	// The backoffs structure is used within the scope of this module and is not
	// scoped to a queue specific class due to the HandlMsg function using it.
	//
	// The TTL cache represents the signal to avoid processing on a queue, think
	// of it as a negative signal that has an expiry time.
	//
	// Create a cache with a default expiration time of 1 minute, and which
	// purges expired items every 10 seconds
	//
	backoffs *runner.Backoffs

	refreshSuccesses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_refresh_success",
			Help: "Number of successful queue inventory checks.",
		},
		[]string{"host", "project"},
	)
	refreshFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_refresh_fail",
			Help: "Number of failed queue inventory checks.",
		},
		[]string{"host", "project"},
	)

	queueChecked = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_checked",
			Help: "Number of times a queue is queried for work.",
		},
		[]string{"host", "queue_type", "queue_name"},
	)
	queueIgnored = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_queue_ignored",
			Help: "Number of times a queue is intentionally not queried, or skipped work.",
		},
		[]string{"host", "queue_type", "queue_name"},
	)
	queueRunning = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_project_running",
			Help: "Number of experiments being actively worked on per queue.",
		},
		[]string{"host", "queue_type", "queue_name", "project", "experiment"},
	)
	queueRan = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "runner_project_completed",
			Help: "Number of experiments that have been run per queue.",
		},
		[]string{"host", "queue_type", "queue_name", "project", "experiment"},
	)

	host = runner.GetHostName()
)

func init() {
	prometheus.MustRegister(refreshSuccesses)
	prometheus.MustRegister(refreshFailures)
	prometheus.MustRegister(queueChecked)
	prometheus.MustRegister(queueIgnored)
	prometheus.MustRegister(queueRunning)
	prometheus.MustRegister(queueRan)
}

// SubsBusy is used to track subscriptions and queues that are currently being actively serviced
// by this runner
type SubsBusy struct {
	subs map[string]bool // The catalog of all known queues (subscriptions) within the project this server is handling
	sync.Mutex
}

// Queuer stores the data associated with a runner instances of a queue worker at the level of the queue itself
//
type Queuer struct {
	project string        // The project that is being used to access available work queues
	cred    string        // The credentials file associated with this project
	subs    Subscriptions // The subscriptions that exist within this project
	busyQs  SubsBusy
	timeout time.Duration // The queue query timeout
	tasker  runner.TaskQueue
}

// SubRequest encapsulates the simple access details for a subscription.  This structure
// is used by the server to send requests that the queue be examined for work.
//
type SubRequest struct {
	project      string
	subscription string
	creds        string
}

// NewQueuer will create a new task queue that will process the queue using the
// returned qr receiver
//
func NewQueuer(projectID string, creds string) (qr *Queuer, err kv.Error) {
	qr = &Queuer{
		project: projectID,
		cred:    creds,
		subs:    Subscriptions{subs: map[string]*Subscription{}},
		busyQs:  SubsBusy{subs: map[string]bool{}},
		timeout: 15 * time.Second,
	}
	qr.tasker, err = runner.NewTaskQueue(projectID, creds)
	if err != nil {
		return nil, err
	}
	return qr, nil
}

// refresh is used to update the queuer with a list of available queues
// accessible to the project specified by the queuer
//
func (qr *Queuer) refresh() (err kv.Error) {

	ctx, cancel := context.WithTimeout(context.Background(), qr.timeout)
	defer cancel()

	matcher, errGo := regexp.Compile(*queueMatch)
	if errGo != nil {
		if len(*queueMatch) != 0 {
			logger.Warn(kv.Wrap(errGo).With("matcher", *queueMatch).With("stack", stack.Trace().TrimRuntime()).Error())
		}
		matcher = nil
	}

	// If the length of the mismatcher is 0 then we will get a nil and because this
	// was checked in the main we can ignore that as this is optional
	mismatcher := &regexp.Regexp{}

	if len(strings.Trim(*queueMismatch, " \n\r\t")) == 0 {
		mismatcher = nil
	} else {
		mismatcher, errGo = regexp.Compile(*queueMismatch)
		if errGo != nil {
			logger.Warn(kv.Wrap(errGo).With("mismatcher", *queueMismatch).With("stack", stack.Trace().TrimRuntime()).Error())
			mismatcher = nil
		}
	}

	known, err := qr.tasker.Refresh(ctx, matcher, mismatcher)
	if err != nil {
		refreshFailures.With(prometheus.Labels{"host": host, "project": qr.project}).Inc()
		return err
	}
	refreshSuccesses.With(prometheus.Labels{"host": host, "project": qr.project}).Inc()

	if logger.IsDebug() {
		keys := []string{}
		for k, _ := range known {
			keys = append(keys, k)
		}
		logger.Debug("known queues", "known", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
		keys = []string{}
		for k, _ := range qr.subs.subs {
			keys = append(keys, k)
		}
		logger.Debug("subscribed queues", "qr.subs.subs", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
	}

	// Bring the queues collection uptodate with what the system has in terms
	// of functioning queues
	//
	added, removed := qr.subs.align(known)
	for _, add := range added {
		logger.Debug("added queue", "queue", add, "stack", stack.Trace().TrimRuntime())
	}
	for _, remove := range removed {
		logger.Debug("removed queue", "queue", remove, "stack", stack.Trace().TrimRuntime())
	}
	return nil
}

// producer is used to examine the subscriptions that are available and determine if
// capacity is available to service any of the work that might be waiting
//
func (qr *Queuer) producer(ctx context.Context, rqst chan *SubRequest, interval time.Duration) {

	logger.Debug("started queue producer", "project", qr.project)
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic in producer %#v, %s", r, string(debug.Stack())))
		}

		logger.Debug("stopped queue producer", "project", qr.project)
	}()

	check := time.NewTicker(interval)
	defer check.Stop()

	nextQDbg := time.Now()
	lastQs := 0

	lastReady := time.Now()
	lastReadyAbs := time.Now()

	for {
		select {
		case <-check.C:

			ranked := qr.rank()

			// Some monitoring logging used to tracking traffic on queues
			if logger.IsTrace() {
				if len(ranked) != 0 {
					logger.Trace(fmt.Sprintf("processing %s %d ranked subscriptions %s", qr.project, len(ranked), Spew.Sdump(ranked)))
				} else {
					logger.Trace(fmt.Sprintf("no %s subscriptions found", qr.project))
				}
			} else {
				if logger.IsDebug() {
					// If either the queue length has changed, or sometime has passed since
					// the last debug log, one minute, print the queue checking state
					if nextQDbg.Before(time.Now()) || lastQs != len(ranked) {
						lastQs = len(ranked)
						nextQDbg = time.Now().Add(10 * time.Minute)
						if len(ranked) == 0 {
							logger.Debug(fmt.Sprintf("no %s subscriptions found", qr.project))
						}
					}
				}
			}

			// track the first queue that has not been checked for the longest period of time that
			// also has no traffic on this node.  This queue will be check but it wont be until the next
			// pass that a new empty or idle queue will be checked.
			idle := []Subscription{}

			for _, sub := range ranked {
				// IDLE queue processing, that is queues that have no work running
				// against this runner
				if sub.cnt == 0 {
					if _, isPresent := backoffs.Get(qr.project + ":" + sub.name); !isPresent {
						idle = append(idle, sub)
					} else {
						logger.Trace(fmt.Sprintf("backed off %s:%s", qr.project, sub.name), "stack", stack.Trace().TrimRuntime())
					}
					// Save the queue that has been waiting the longest into the
					// idle slot that we will be processing on this pass
				}
			}

			if len(idle) != 0 {

				// Shuffle the queues to pick one at random, fisher yates shuffle introduced in
				// go 1.10, c.f. https://golang.org/pkg/math/rand/#Shuffle
				rand.Shuffle(len(idle), func(i, j int) {
					idle[i], idle[j] = idle[j], idle[i]
				})

				if err := qr.check(ctx, idle[0].name, rqst); err != nil {

					backoffs.Set(qr.project+":"+idle[0].name, time.Duration(time.Minute))

					logger.Warn(fmt.Sprintf("checking %s for work failed due to %s, backoff 1 minute", qr.project+":"+idle[0].name, err.Error()))
					break
				}
				lastReady = time.Now()
				lastReadyAbs = time.Now()
			}

			// Check to see if we were last ready for work more than one hour ago as
			// this could be a resource problem
			if lastReady.Before(time.Now().Add(-1 * time.Hour)) {
				// If we have been unavailable for work alter slack once every 10 minutes and then
				// bump the ready timer for wait for another 10 before resending the advisory
				lastReady = lastReady.Add(10 * time.Minute)
				logger.Warn("this host has been idle for a long period of time please check for disk space etc resource availability",
					"idleTime", time.Now().Sub(lastReadyAbs))
			}
		case <-ctx.Done():
			return
		}
	}
}

func (qr *Queuer) getResources(name string) (rsc *runner.Resource) {
	qr.subs.Lock()
	defer qr.subs.Unlock()

	item, isPresent := qr.subs.subs[name]
	if !isPresent || item.rsc == nil {
		return nil
	}

	return item.rsc.Clone()
}

// Retrieve the queues and count their occupancy, then sort ascending into
// an array
func (qr *Queuer) rank() (ranked []Subscription) {
	qr.subs.Lock()
	defer qr.subs.Unlock()

	ranked = make([]Subscription, 0, len(qr.subs.subs))
	for _, sub := range qr.subs.subs {
		ranked = append(ranked, *sub)
	}

	// sort the queues by their frequency of work, not their occupany of resources
	// so this is approximate but good enough for now
	//
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].cnt < ranked[j].cnt })

	return ranked
}

// check will first validate a subscription and will add it to the list of subscriptions
// to be processed, which is in turn used by the scheduler later.
//
func (qr *Queuer) check(ctx context.Context, name string, rQ chan *SubRequest) (err kv.Error) {

	if rsc := qr.getResources(name); rsc != nil {
		// In the event we know the resource requirements of requests that will appear on a given
		// subscription we can first check if there is any chance of the working being processed
		// and if not stop early.
		if fit, err := rsc.Fit(getMachineResources()); !fit {
			if err != nil {
				return err
			}

			if logger.IsTrace() {
				logger.Trace("no fit", "project", qr.project, "subscription", name, "rsc", rsc, "headroom", getMachineResources(),
					"stack", stack.Trace().TrimRuntime())
			}
			return nil
		}
		if logger.IsTrace() {
			logger.Trace("passed capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
		}
	} else {
		if logger.IsTrace() {
			logger.Trace("skipped capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
		}
	}

	// Check to see if anyone is listening for a queue to check by sending a dummy request, and then
	// send the real request if the check message is consumed
	select {
	case rQ <- &SubRequest{}:
	default:
		return kv.NewError("busy consumer, at the 1ˢᵗ stage").With("stack", stack.Trace().TrimRuntime())
	}

	select {
	// Enough needs to be sent at this point that the queue could be found and checked
	// by the message queue handling implementation
	case rQ <- &SubRequest{project: qr.project, subscription: name, creds: qr.cred}:
	case <-time.After(2 * time.Second):
		return kv.NewError("busy checking consumer, at the 2ⁿᵈ stage").With("stack", stack.Trace().TrimRuntime())
	}

	return nil
}

// run will execute maintenance operations in the back ground for the server looking for new
// or old subscriptions and adding them or removing them as needed
//
// This function will block except in the case a fatal error occurs that prevents it
// from being able to perform the function that it is intended to do
//
func (qr *Queuer) run(ctx context.Context, refreshQueues time.Duration, workChecking time.Duration) (err kv.Error) {

	// Start a worker which accepts queue checking requests via a channel, when the worker
	// is occupied the channel will block.  The producer can check the consumer has
	// the capacity for queue processing by sending a test message down the channel.
	sendWork := make(chan *SubRequest)
	go qr.consumer(ctx, sendWork)

	// start a producer that looks at subscriptions and then checks the
	// sendWork listener to ensure there is capacity before sending the
	// request that a specific queue be checked via a channel

	go qr.producer(ctx, sendWork, workChecking)

	refresh := time.Duration(time.Second)

	for {
		select {
		case <-time.After(refresh):
			if err := qr.refresh(); err != nil {
				return err
			}
			// Check for new queues or deleted queues once every few minutes
			refresh = time.Duration(refreshQueues)
		case <-ctx.Done():
			return nil
		}
	}
}

func (qr *Queuer) consumer(ctx context.Context, readyC chan *SubRequest) {

	logger.Debug("started queue consumer", "project", qr.project)

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic in consumer %#v, %s", r, string(debug.Stack())))
		}

		logger.Debug("stopped queue consumer", "project", qr.project)
	}()

	for {
		select {
		case request := <-readyC:
			// The channel looks to have been closed so stop handling work
			if request == nil {
				return
			}
			// An empty structure will be sent when the sender want to check if
			// the worker is ready for a scheduling request for a queue
			if len(request.subscription) == 0 {
				continue
			}

			go qr.filterWork(ctx, request)
		case <-ctx.Done():
			return
		}
	}
}

// filterWork handles requests to check queues/subscriptions for work.
//
// Before checking it will ensure that a backoff time is not in play
// for the subscription, if it is then it will simply return.
//
// This method also checks that the subscription is not already being
// processed concurrently.
//
// This receiver blocks until the ctx it is passed is Done().
//
func (qr *Queuer) filterWork(ctx context.Context, request *SubRequest) {

	if _, isPresent := backoffs.Get(request.project + ":" + request.subscription); isPresent {
		logger.Trace(fmt.Sprintf("backoff on for %v", request))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic in filterWork %#v, %s", r, string(debug.Stack())))
		}
	}()

	qr.busyQs.Lock()
	_, busy := qr.busyQs.subs[request.subscription]
	if !busy {
		qr.busyQs.subs[request.subscription] = true
	}
	qr.busyQs.Unlock()

	if busy {
		return
	}

	defer func() {
		qr.busyQs.Lock()
		delete(qr.busyQs.subs, request.subscription)
		qr.busyQs.Unlock()
	}()

	qr.doWork(ctx, request)
}

// doWork will dispatch a message handler on behalf of a queue via the queues Work(...) method
// passing down a context to signal the worker when the world of that queue has come to its end.
//
// This function blocks until it has been signalled that the queue with which it is associated has
// stopped processing.  This is done via the passed in ctx parameter.
//
// This receiver will spin off a thread for the queue specific implementation of the Work(...)
// method.
//
// The lifetime of this listener for queue work is intended to stretch for the lifetime of the
// queue itself.
//
func (qr *Queuer) doWork(ctx context.Context, request *SubRequest) {

	if _, isPresent := backoffs.Get(request.project + ":" + request.subscription); isPresent {
		logger.Trace(fmt.Sprintf("%v, backed off", request))
		return
	}

	logger.Debug("started doWork", "project_id", request.project, "subscription_id", request.subscription)
	defer logger.Debug("completed doWork", "project_id", request.project, "subscription_id", request.subscription)

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic running studioml script %#v, %s", r, string(debug.Stack())))
		}
	}()

	// cCtx is used to cancel any workers when a queue disappears
	cCtx, workCancel := context.WithCancel(context.Background())
	defer workCancel()

	go func() {

		// Spins out a go routine to handle messages, HandleMsg will be invoked
		// by the queue specific implementation in the event that valid work is found
		// which is typically done via the queues Work(...) method
		//
		qt := &runner.QueueTask{
			FQProject:    qr.project,
			Project:      request.project,
			Subscription: request.subscription,
			Handler:      HandleMsg,
		}

		check := time.NewTicker(10 * time.Second)
		defer check.Stop()

		// A long lived polling loop scanning for work, it will dispatch work for a single queue server
		// at most once every 10 seconds.  The backoffs structure will be use to throttle the dispatcher
		// for longer periods of idle time.
		for {
			select {
			case <-check.C:
				if _, isPresent := backoffs.Get(request.project + ":" + request.subscription); isPresent {
					continue
				}

				// Invoke the work handling in a go routine to allow other work
				// to be scheduled
				go qr.fetchWork(cCtx, qt)
			case <-cCtx.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// While the above func is looking for work check periodically that
	// the queue that was used to send the message still exists, if it
	// does not cancel everything as this is an indication that the
	// work is intended to be aburptly terminated.
	func() {
		check := time.NewTicker(5 * time.Minute)
		defer check.Stop()

		for {
			select {
			case <-check.C:
				eCtx, eCancel := context.WithTimeout(context.Background(), qr.timeout)
				// Is the queue still there that the job came in on, TODO the state information
				// can be obtainer from the queue refresher in the refresh() function
				exists, err := qr.tasker.Exists(eCtx, request.subscription)
				eCancel()

				if err != nil {
					logger.Info(fmt.Sprintf("%s:%s could not be validated due to %s", request.project, request.subscription, err))
					continue
				}
				if !exists {
					logger.Warn(fmt.Sprintf("%s:%s no longer found cancelling running tasks", request.project, request.subscription))
					// If not simply return which will cancel the context being used to manage the
					// lifecycle of task processing
					return
				}
				logger.Debug("doWork alive", "project_id", request.project, "subscription_id", request.subscription)

			case <-cCtx.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// fetchWork will use the queue specific implementation for retriving a single work item
// if the queue has any and will block while the work is done.  If no work is available
// it will return.
//
func (qr *Queuer) fetchWork(ctx context.Context, qt *runner.QueueTask) {
	// Increment the inflight counter for the worker
	qr.subs.incWorkers(qt.Subscription)
	// Use the context for workers that is canceled once a queue disappears
	processed, rsc, err := qr.tasker.Work(ctx, qt)
	// Decrement the inflight counter for the worker
	qr.subs.decWorkers(qt.Subscription)

	// No work found return to waiting for some
	if !processed || err != nil {
		backoffTime := time.Duration(2 * time.Minute)
		backoffs.Set(qt.Project+":"+qt.Subscription, backoffTime)

		if err != nil {
			logger.Warn("backing off, receive failed", "duration", backoffTime, "project_id", qt.Project, "subscription_id", qt.Subscription, "error", strings.Replace(err.Error(), "\n", "", 0))
		} else {
			logger.Debug("backing off", "duration", backoffTime, "project_id", qt.Project, "subscription_id", qt.Subscription)
		}
		return
	}

	// Set the default resource requirements for the next message fetch to that of the most recently
	// seen resource request
	//
	if rsc != nil {
		if err := qr.subs.setResources(qt.Subscription, rsc); err != nil {
			logger.Info("resource updated failed", "project_id", qt.Project, "subscription_id", qt.Subscription, "error", err.Error())
		}
	}
}
