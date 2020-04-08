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
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/mgutz/logxi"

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

	// queuePollInterval is used for polling the queue server for work
	queuePollInterval = time.Duration(10 * time.Second)

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

	backoffs = runner.GetBackoffs()
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
		subs: Subscriptions{
			subs: map[string]*Subscription{},
		},
		busyQs:  SubsBusy{subs: map[string]bool{}},
		timeout: 15 * time.Second,
	}
	qr.tasker, err = runner.NewTaskQueue(projectID, creds)
	if err != nil {
		return nil, err
	}
	return qr, nil
}

// refresh is used to update the queuer with a list of the available queues
// accessible to the project
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
	_ = mismatcher // Bypass the ineffectual assignment check

	if len(strings.Trim(*queueMismatch, " \n\r\t")) == 0 {
		mismatcher = nil
	} else {
		mismatcher, errGo = regexp.Compile(*queueMismatch)
		if errGo != nil {
			logger.Warn(kv.Wrap(errGo).With("mismatcher", *queueMismatch).With("stack", stack.Trace().TrimRuntime()).Error())
			mismatcher = nil
		}
	}

	// When asking the queue server specific implementation of a directory of
	// the queues it knows about we supply regular expressions to filter the
	// results
	known, err := qr.tasker.Refresh(ctx, matcher, mismatcher)
	if err != nil {
		refreshFailures.With(prometheus.Labels{"host": host, "project": qr.project}).Inc()
		return err
	}
	refreshSuccesses.With(prometheus.Labels{"host": host, "project": qr.project}).Inc()

	if logger.IsDebug() {
		keys := []string{}
		for k := range known {
			keys = append(keys, k)
		}
		logger.Debug("known queues", "known", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
		keys = []string{}
		for k := range qr.subs.subs {
			keys = append(keys, k)
		}
		logger.Debug("subscribed queues", "qr.subs.subs", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
	}

	// Bring the queues collection uptodate with what the system has in terms
	// of functioning queues
	//
	added, removed := qr.subs.align(known)
	if logger.IsDebug() {
		for _, add := range added {
			logger.Debug("added queue", "queue", add, "stack", stack.Trace().TrimRuntime())
		}
		for _, remove := range removed {
			logger.Debug("removed queue", "queue", remove, "stack", stack.Trace().TrimRuntime())
		}
	}
	return nil
}

// producer is used to examine the subscriptions that are available and determine if
// capacity is available to service any of the work that might be waiting
//
func (qr *Queuer) producer(ctx context.Context, interval time.Duration) {

	logger.Debug("started queue producer", "project", qr.project)
	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic in producer %#v, %s", r, string(debug.Stack())))
		}

		logger.Debug("stopped queue producer", "project", qr.project)
	}()

	check := time.NewTicker(interval)
	defer check.Stop()

	// On a regular timer check the list of queues for the server and see if
	// any need a worker started
	for {
		select {
		case <-check.C:

			for _, sub := range qr.getSubscriptions() {

				qr.busyQs.Lock()
				_, busy := qr.busyQs.subs[sub.name]
				qr.busyQs.Unlock()

				// We already have a worker running for this specific queue
				if busy {
					continue
				}

				// check will send the queue information to the consumer to be used
				// to start a go routine that services it, only if the queue is
				// not already being processed
				if capacityOK, err := qr.check(ctx, sub.name); err != nil {
					logger.Warn(fmt.Sprintf("checking %s for work failed due to %s, backoff 1 minute", qr.project+":"+sub.name, err.Error()))
					break
				} else {
					if capacityOK {
						request := &SubRequest{project: qr.project, subscription: sub.name, creds: qr.cred}

						go qr.filterWork(ctx, request)
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// geResources will retrieve a copy of the data used to describe the resource
// requirements of a queue
//
func (qr *Queuer) getResources(name string) (rsc *runner.Resource) {
	qr.subs.Lock()
	defer qr.subs.Unlock()

	item, isPresent := qr.subs.subs[name]
	if !isPresent || item.rsc == nil {
		return nil
	}

	return item.rsc.Clone()
}

// getSubscriptions will retrieve the queues active within the server and return a copy of
// them in an array.
func (qr *Queuer) getSubscriptions() (copied []Subscription) {
	qr.subs.Lock()
	defer qr.subs.Unlock()

	copied = make([]Subscription, 0, len(qr.subs.subs))
	// The following is a map and traversed in an undetermined order to reduce
	// side effects in processing the queues
	for _, sub := range qr.subs.subs {
		copied = append(copied, *sub)
	}

	return copied
}

// check will first validate that the potential work to be performed can indeed be done
// and if so will dispatch the queue processing for it
//
func (qr *Queuer) check(ctx context.Context, name string) (capacity bool, err kv.Error) {

	if rsc := qr.getResources(name); rsc != nil {
		// In the event we know the resource requirements of requests that will appear on a given
		// subscription we can first check if there is any chance of the working being processed
		// and if not stop early.
		if fit, err := rsc.Fit(getMachineResources()); !fit {
			if err != nil {
				return false, err
			}

			if logger.IsTrace() {
				logger.Trace("no fit", "project", qr.project, "subscription", name, "rsc", rsc, "headroom", getMachineResources(),
					"stack", stack.Trace().TrimRuntime())
			}
			return false, nil
		}
	} else {
		if logger.IsTrace() {
			logger.Trace("skipped capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
		}
	}

	if logger.IsTrace() {
		logger.Trace("passed capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
	}
	return true, nil
}

// run will execute maintenance operations in the back ground for the server looking for new
// or old subscriptions and adding them or removing them as needed
//
// This function will block except in the case a fatal error occurs that prevents it
// from being able to perform the function that it is intended to do
//
func (qr *Queuer) run(ctx context.Context, refreshQueues time.Duration, workChecking time.Duration) (err kv.Error) {

	// start a producer that looks at subscriptions and then checks the
	// sendWork listener to ensure there is capacity before sending the
	// request that a specific queue be checked via a channel
	//
	go qr.producer(ctx, workChecking)

	// Now start a queue server refresher that will be called to obtain the latest list
	// of known queues in the system
	//
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
		logger.Trace("backoff on", "project_id", request.project, "subscription_id", request.subscription)
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

		// Store what the polling interval was last set to in order that when longer polls
		// are used to eat up backoff time we can reset to the standard value for the ticker
		pollDuration := queuePollInterval
		check := time.NewTicker(pollDuration)
		defer check.Stop()

		// A long lived polling loop scanning for work, it will dispatch work for a single queue server
		// at most once every 10 seconds.  The backoffs structure will be use to throttle the dispatcher
		// for longer periods of idle time.
		for {
			select {
			case <-check.C:
				if delayUntil, isPresent := backoffs.Get(request.project + ":" + request.subscription); isPresent {
					delayLeft := delayUntil.Sub(time.Now())
					if delayLeft >= 0 {
						// Take a single tick into the future to when the backoff will be done
						pollDuration = delayLeft
						check.Stop()
						check = time.NewTicker(pollDuration)
					}
					continue
				}

				// Invoke the work handling in a go routine to allow other work
				// to be scheduled
				go qr.fetchWork(cCtx, qt)

				// If the last tick was a non standard one then change back to a standard polling
				// interval
				if pollDuration != queuePollInterval {
					pollDuration = queuePollInterval
					check.Stop()
					check = time.NewTicker(pollDuration)
				}
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
	// work is intended to be abruptly terminated.
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

	// If we are able to determine the required capacity for the queue and
	// the node does not have sufficient available dont both going to get any
	// work
	capacityOK, err := qr.check(ctx, qt.Subscription)
	if err != nil {
		capacityOK = true
	}

	workDone := false
	startedAt := time.Now()

	if capacityOK {

		// Increment the inflight counter for the worker
		qr.subs.incWorkers(qt.Subscription)
		// Use the context for workers that is canceled once a queue disappears
		processed, rsc, qErr := qr.tasker.Work(ctx, qt)
		// Decrement the inflight counter for the worker
		qr.subs.decWorkers(qt.Subscription)

		// Set the default resource requirements for the next message fetch to that of the most recently
		// seen resource request
		//
		if rsc != nil {
			if err := qr.subs.setResources(qt.Subscription, rsc); err != nil {
				logger.Info("resource updated failed", "project_id", qt.Project, "subscription_id", qt.Subscription, "error", err.Error())
			}
		}

		workDone = processed
		err = qErr
	}

	// As jobs finish we should determine what they delay should be before the
	// runner should look for the next job in the specific queue being used
	// should be.  Thisd acts as a form of penalty for queuing new work based on
	// how long the jobs are taking and if errors are occurring in them.  We start
	// assuming that a 2 minute penalty exists to cover the worst case penalty.
	backoffTime := time.Duration(2 * time.Minute)
	msg := "backing off"
	lvl := logxi.LevelDebug
	msgVars := []interface{}{"project_id", qt.Project, "subscription_id", qt.Subscription}

	if err != nil {
		// No work found return to waiting for some, at the outer bound of the queue service
		// interval
		lvl = logxi.LevelWarn
		msg = msg + ", receive failed"
		msgVars = append(msgVars, "error", err.Error())
	} else {

		if !workDone && capacityOK {
			msg = msg + ", empty"
		}
		if !capacityOK {
			msg = msg + ", no capacity"
		}

		// Only if work was actually done do we add a measurement to the EMA
		if workDone {
			// Take the execution duration and use it to calculate a relative penalty for
			// new jobs being queued.  This allows smaller requests to sneak through while
			// the larger projects are paying the penalty in the form of a backoff.
			execTime := time.Now().Sub(startedAt)
			qr.subs.execTime(qt.Subscription, execTime)
		}

		// If we dont have a backoff in effect use the average run time to penalize
		// ourselves for the next attempt at queuing work, only do this if we are
		// not already is a backoff situation otherwise backoffs will just keep piling
		// up
		if avg, err := qr.subs.getExecAvg(qt.Subscription); err != nil {
			logger.Warn("could not calculate execution time", "project_id", qr.project, "subscription_id", qt.Subscription, "error", err.Error())
		} else {
			backoffTime = time.Duration(time.Duration(avg.Hours()/2.0) * queuePollInterval)
		}
	}

	// Set the penalty for the queue, except where one is already in effect
	if delayed, isPresent := backoffs.Get(qr.project + ":" + qt.Subscription); !isPresent {
		// Use a default of 15 seconds if no backoff has been specified
		if backoffTime == time.Duration(0) {
			backoffTime = time.Duration(15 * time.Second)
		}
		backoffs.Set(qt.Project+":"+qt.Subscription, backoffTime)
		msg = msg + ", now delayed"
	} else {
		msg = msg + ", already delayed"
		backoffTime = delayed.Sub(time.Now()).Truncate(time.Second)
	}

	msgVars = append(msgVars, "duration", backoffTime.String())
	logger.Log(lvl, msg, msgVars)
}
