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
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/golang/protobuf/ptypes/wrappers"

	"github.com/leaf-ai/go-service/pkg/network"
	"github.com/leaf-ai/go-service/pkg/server"
	aws_ext "github.com/leaf-ai/studio-go-runner/pkg/aws"
	"github.com/leaf-ai/studio-go-runner/pkg/wrapper"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"
	"github.com/leaf-ai/studio-go-runner/internal/resources"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/task"

	logxi "github.com/karlmutch/logxi/v1"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/prometheus/client_golang/prometheus"
)

const (
	responseSuffix = "_response"
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

	host = network.GetHostName()
)

func init() {
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
	tasker  task.TaskQueue
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
func NewQueuer(projectID string, mgt string, creds string, w wrapper.Wrapper) (qr *Queuer, err kv.Error) {
	qr = &Queuer{
		project: projectID,
		cred:    creds,
		subs: Subscriptions{
			subs: map[string]*Subscription{},
		},
		busyQs:  SubsBusy{subs: map[string]bool{}},
		timeout: 15 * time.Second,
	}
	qr.tasker, err = NewTaskQueue(projectID, mgt, creds, w)
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

	// Ignore queues used for response messages
	for k := range known {
		if strings.HasSuffix(k, responseSuffix) {
			delete(known, k)
		}
	}
	added, removed := qr.subs.align(known)

	if logger.IsDebug() {
		qr.reportQChanges(known, added, removed)
	}
	return nil
}

func (qr *Queuer) reportQChanges(known map[string]interface{}, added []string, removed []string) {
	if logger.IsTrace() {
		keys := []string{}
		for k := range known {
			keys = append(keys, k)
		}
		logger.Trace("known queues", "known", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
		keys = []string{}
		for k := range qr.subs.subs {
			keys = append(keys, k)
			logger.Trace("subscribed queues", "qr.subs.subs", strings.Replace(spew.Sdump(keys), "\n", ", ", -1))
		}
	}

	// Bring the queues collection uptodate with what the system has in terms
	// of functioning queues
	//
	for _, add := range added {
		logger.Debug("added queue", "queue", add, "stack", stack.Trace().TrimRuntime())
	}
	for _, remove := range removed {
		logger.Debug("removed queue", "queue", remove, "stack", stack.Trace().TrimRuntime())
	}
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

			for _, sub := range qr.subscriptions() {

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
				if capacityMaybe, err := qr.check(ctx, sub.name); err != nil {
					logger.Warn(fmt.Sprintf("checking %s for work failed due to %s, backoff %s", qr.project+":"+sub.name, err.Error(), interval))
					break
				} else {
					if capacityMaybe {
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

// resources will retrieve a copy of the data used to describe the resource
// requirements of a queue
//
func (qr *Queuer) resources(name string) (rsc *server.Resource) {
	qr.subs.Lock()
	defer qr.subs.Unlock()

	item, isPresent := qr.subs.subs[name]
	if !isPresent || item.rsc == nil {
		if isPresent {
			logger.Trace("subscription has no resource", "name", name)
		} else {
			logger.Warn("subscription is missing", "name", name, "stack", stack.Trace().TrimRuntime())
		}
		return nil
	}

	return item.rsc.Clone()
}

// subscriptions will retrieve the queues active within the server and return a copy of
// them in an array.
func (qr *Queuer) subscriptions() (copied []Subscription) {
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
// or we dont know as we dont have enough information yet dispatch the queue processing for it
//
func (qr *Queuer) check(ctx context.Context, name string) (capacity bool, err kv.Error) {

	isTrace := logger.IsTrace()

	machineRcs := (&resources.Resources{}).FetchMachineResources()

	if rsc := qr.resources(name); rsc != nil {
		// In the event we know the resource requirements of requests that will appear on a given
		// subscription we can first check if there is any chance of the working being processed
		// and if not stop early.
		if fit, err := rsc.Fit(machineRcs); !fit {
			if err != nil {
				return false, err
			}

			if isTrace {
				logger.Trace("no fit", "project", qr.project, "subscription", name, "rsc", rsc, "headroom", machineRcs,
					"stack", stack.Trace().TrimRuntime())
			}
			return false, nil
		}
		if isTrace {
			logger.Trace("passed capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
		}
	} else {
		if isTrace {
			logger.Trace("skipped capacity check", "project", qr.project, "subscription", name, "stack", stack.Trace().TrimRuntime())
		}
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

// startFetch polls a queue for work using a request block that contains information
// about the queue.
//
// startFetch invokes fetchWork function and run as a go func. It is called by doWork.
//
func (qr *Queuer) startFetch(ctx context.Context, request *SubRequest) {

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
				delayLeft := time.Until(delayUntil)
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
			go func() {
				w, err := getWrapper()
				if err != nil {
					logger.Debug("encryption wrapper skipped", "error", err)
				}

				// Create a different QueueTask for every attempt to schedule a single work item
				qt := &task.QueueTask{
					FQProject:    qr.project,
					Project:      request.project,
					Subscription: request.subscription,
					Handler:      HandleMsg,
					Wrapper:      w,
				}

				qr.fetchWork(ctx, qt)
			}()

			// If the last tick was a non standard one then change back to a standard polling
			// interval
			if pollDuration != queuePollInterval {
				pollDuration = queuePollInterval
				check.Stop()
				check = time.NewTicker(pollDuration)
			}
		case <-ctx.Done():
			return
		}
	}
}

// watchQueueDelete is used to monitor the presence of a queue and if it disappears
// return unblocking the invoking function.
//
func (qr *Queuer) watchQueueDelete(ctx context.Context, cancel context.CancelFunc, request *SubRequest) {
	check := time.NewTicker(5 * time.Minute)
	defer check.Stop()

	terminateAt := time.Unix(0, 0)

	for {
		select {
		case <-check.C:
			eCtx, eCancel := context.WithTimeout(context.Background(), qr.timeout)
			// Is the queue still there that the job came in on, TODO the state information
			// can be obtained from the queue refresher in the refresh() function
			exists, err := qr.tasker.Exists(eCtx, request.subscription)
			eCancel()

			if err != nil {
				logger.Info("queue invalidated", "project_id", request.project, "subscription_id", request.subscription, "error", err)
				continue
			}
			if !exists {
				// Cancel all processing for this queue and also terminate the associated context after a 5 minute cool down
				if terminateAt.Unix() == 0 {
					terminateAt = time.Now().Add(time.Duration(5 * time.Minute))
					continue
				}
				if terminateAt.Before(time.Now()) {
					logger.Warn("queue not found cancelling tasks", "project_id", request.project, "subscription_id", request.subscription)
					cancel()

					return
				}
			}
			terminateAt = time.Unix(0, 0)
			logger.Debug("doWork alive", "project_id", request.project, "subscription_id", request.subscription)

		case <-ctx.Done():
			return
		}
	}
}

func DualWait(ctx context.Context, etx context.Context, cancel context.CancelFunc) {
	defer cancel()
	select {
	case <-ctx.Done():
		return
	case <-etx.Done():
		return
	}
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

	// If any one of the contexts is Done then invoke the cancel function
	go DualWait(ctx, cCtx, workCancel)

	// Spins out a go routine to handle messages, HandleMsg will be invoked
	// by the queue specific implementation in the event that valid work is found
	// which is typically done via the queues Work(...) method
	//
	go qr.startFetch(cCtx, request)

	// While the above func is looking for work check periodically that
	// the queue that was used to send the message still exists, if it
	// does not cancel everything as this is an indication that the
	// work is intended to be abruptly terminated.
	//
	// This function blocks until the context is Done or the queue disappears
	qr.watchQueueDelete(cCtx, workCancel, request)
}

// fetchWork will use the queue specific implementation for retrieving a single work item
// if the queue has any and will block while the work is done.  If no work is available
// it will return.
//
func (qr *Queuer) fetchWork(ctx context.Context, qt *task.QueueTask) {

	// If we are able to determine the required capacity for the queue and
	// the node does not have sufficient available dont both going to get any
	// work
	capacityMaybe, err := qr.check(ctx, qt.Subscription)

	workDone := false
	startedAt := time.Now()

	// Make sure we are not needlessly doing this by seeing if anything at all is waiting
	hasWork, err := qr.tasker.HasWork(ctx, qt.Subscription)

	if hasWork && err == nil {

		if exists, _ := qr.tasker.Exists(ctx, qt.Subscription+responseSuffix); exists {
			shortQueueName, err := qr.tasker.GetShortQName(qt)
			if err != nil {
				logger.Info("no short queue", "error", err.Error)
			} else {
				responseQName := shortQueueName + responseSuffix

				// Check before starting if there is a response queue available for
				// reporting.  If so start a channel for reporting with a listener
				// and a pump to present reports
				if rspEncryptStore := GetRspnsEncrypt(); rspEncryptStore != nil {
					// The response store does not need the response suffice because it only
					// deals with response queue public keys

					if key, err := rspEncryptStore.Select(responseQName); err == nil {

						if responseQ, err := qr.tasker.Responder(ctx, responseQName, key); err != nil {
							logger.Warn("responder unavailable", "queue_name", responseQName, "error", err.Error())
						} else {
							qt.ResponseQ = responseQ
						}
					} else {
						logger.Info("no response key", "queue_name", responseQName, "keys", spew.Sdump(*rspEncryptStore))
					}
				} else {
					logger.Info("no response key store", "queue_name", responseQName)
				}

				if qt.ResponseQ != nil {
					select {
					case qt.ResponseQ <- &runnerReports.Report{
						Time: timestamppb.Now(),
						ExecutorId: &wrappers.StringValue{
							Value: network.GetHostName(),
						},
						Payload: &runnerReports.Report_Logging{
							Logging: &runnerReports.LogEntry{
								Time:     timestamppb.Now(),
								Severity: runnerReports.LogSeverity_Debug,
								Message: &wrappers.StringValue{
									Value: "scanning queue",
								},
								Fields: map[string]string{
									"queue_name": shortQueueName,
								},
							},
						},
					}:
					default:
						// No point responding to back preassure here as recovery
						// is not that important for this type of message
					}
				}
			}
		}

		// Increment the inflight counter for the worker
		qr.subs.incWorkers(qt.Subscription)
		// Use the context for workers that is canceled once a queue disappears
		processed, rsc, qErr := qr.tasker.Work(ctx, qt)
		// Decrement the inflight counter for the worker
		qr.subs.decWorkers(qt.Subscription)

		// Stop the background responder by closing the channel
		if qt.ResponseQ != nil {
			close(qt.ResponseQ)
			qt.ResponseQ = nil
		}

		// Set the default resource requirements for the next message fetch to that of the most recently
		// seen resource request
		//
		if rsc != nil {
			// Update the resources this queue has been requesting per experiment on each new experiment message
			if err := qr.subs.setResources(qt.Subscription, rsc); err != nil {
				logger.Trace("resource update failed", "subscription", qt.Subscription, "error", err.Error())
			} else {
				logger.Debug("resource updated", "subscription", qt.Subscription, "resource", rsc)
			}
		} else {
			logger.Trace("resource update was empty", "subscription", qt.Subscription)
		}

		workDone = processed
		err = qErr
	}

	// As jobs finish we should determine what they delay should be before the
	// runner should look for the next job in the specific queue being used
	// should be.  Thisd acts as a form of penalty for queuing new work based on
	// how long the jobs are taking and if errors are occurring in them.  We start
	// assuming that a 2 minute penalty exists to cover the worst case penalty.
	backoffTime := time.Duration(15 * time.Second)
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

		if !workDone && capacityMaybe {
			lvl = logxi.LevelTrace
			msg = msg + ", empty"
		}
		if !capacityMaybe {
			msg = msg + ", no capacity"
			if err != nil {
				msg = msg + ", " + err.Error()
			}
		}

		// Only if work was actually done do we add a measurement to the EMA
		if workDone {
			// Take the execution duration and use it to calculate a relative penalty for
			// new jobs being queued.  This allows smaller requests to sneak through while
			// the larger projects are paying the penalty in the form of a backoff.
			execTime := time.Since(startedAt)
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
		lvl = logxi.LevelTrace
		msg = msg + ", already delayed"
		backoffTime = time.Until(delayed).Truncate(time.Second)
	}

	msgVars = append([]interface{}{"duration", backoffTime.String()}, msgVars...)
	logger.Log(lvl, msg, msgVars)
}

// NewTaskQueue is used to initiate processing for any of the types of queues
// the runner supports.  It also performs some lazy initialization.
//
func NewTaskQueue(project string, mgt string, creds string, w wrapper.Wrapper) (tq task.TaskQueue, err kv.Error) {

	switch {
	case strings.HasPrefix(project, "amqp://"), strings.HasPrefix(project, "amqps://"):
		tq, err = runner.NewRabbitMQ(project, mgt, creds, w, logger)
	default:
		// SQS uses a number of credential and config file names
		files := strings.Split(creds, ",")
		for _, file := range files {
			_, errGo := os.Stat(file)
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file).With("project", project)
			}
		}
		tq, err = aws_ext.NewSQS(project, creds, w)
	}

	return tq, err
}
