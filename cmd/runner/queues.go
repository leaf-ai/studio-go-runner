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

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/types"

	"github.com/dustin/go-humanize" // MIT License
	uberatomic "go.uber.org/atomic" // MIT License

	"github.com/karlmutch/go-cache"

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
	// The TTL cache represents the signal to not do something, think of it as a
	// negative signal that has an expiry time.
	//
	// Create a cache with a default expiration time of 1 minute, and which
	// purges expired items every 10 seconds
	//
	backoffs = cache.New(10*time.Second, time.Minute)

	// busyQs is used to indicate when a worker is active for a named project:subscription so
	// that only one worker is activate at a time
	//
	busyQs = SubsBusy{subs: map[string]bool{}}

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

	k8sOnceListener sync.Once
	openForBiz      = uberatomic.NewBool(true)

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

// Projects is used across several queuing modules for example the google pubsub and the rabbitMQ modules
//
type Projects struct {
	queueType string
	projects  map[string]context.CancelFunc
	sync.Mutex
}

func (*Projects) startStateWatcher(ctx context.Context) (err kv.Error) {
	lifecycleC := make(chan runner.K8sStateUpdate, 1)
	id, err := k8sStateUpdates().Add(lifecycleC)
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			k8sStateUpdates().Delete(id)
			close(lifecycleC)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case state := <-lifecycleC:
				openForBiz.Store(state.State == types.K8sRunning)
			}
		}
	}()

	return err
}

// Lifecycle is used to run a single pass across all of the found queues and subscriptions
// quering for work and any needed updates to the list of queues found within the various queue
// servers that are configured
//
// live has a list of queue references as determined by the queue implementation
// found has a map of queue references specific to the queue implementation, the key, and
// a value with credential information
//
func (live *Projects) Lifecycle(ctx context.Context, found map[string]string) (err kv.Error) {

	if len(found) == 0 {
		return nil
	}

	if !openForBiz.Load() {
		return nil
	}

	k8sOnceListener.Do(func() {
		err = live.startStateWatcher(ctx)
	})

	if err != nil {
		return err
	}

	// If projects have disappeared from the credentials then kill them from the
	// running set of projects if they are still running
	live.Lock()
	for proj, quiter := range live.projects {
		if _, isPresent := found[proj]; !isPresent {
			quiter()
			delete(live.projects, proj)
			logger.Info(proj+" no longer available", "stack", stack.Trace().TrimRuntime())
		}
	}
	live.Unlock()

	// Having checked for projects that have been dropped look for new projects
	for proj, cred := range found {

		logger.Trace("Lifecycle "+proj, "stack", stack.Trace().TrimRuntime())
		queueChecked.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": proj}).Inc()

		live.Lock()
		if _, isPresent := live.projects[proj]; !isPresent {

			// Now start processing the queues that exist within the project in the background
			qr, err := NewQueuer(proj, cred)
			if err != nil {
				logger.Warn(err.Error())
				live.Unlock()
				continue
			}
			ctx, cancel := context.WithCancel(context.Background())
			live.projects[proj] = cancel

			// Start the projects runner and let it go off and do its thing until it dies
			// for no longer has a matching credentials file
			go func(ctx context.Context, proj string) {
				logger.Debug("queue runner processing", "project_id", proj,
					"stack", stack.Trace().TrimRuntime())

				if err := qr.run(ctx, 5*time.Minute); err != nil {
					logger.Warn("queue runner failed", "project", proj, "error", err)
				}

				live.Lock()
				delete(live.projects, proj)
				live.Unlock()
			}(ctx, proj)
		}
		live.Unlock()
	}

	return err
}

// SubsBusy is used to track subscriptions and queues that are currently being actively serviced
// by this runner
type SubsBusy struct {
	subs map[string]bool // The catalog of all known queues (subscriptions) within the project this server is handling
	sync.Mutex
}

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

// Queuer stores the data associated with a runner instances of a queue worker at the level of the queue itself
//
type Queuer struct {
	project string        // The project that is being used to access available work queues
	cred    string        // The credentials file associated with this project
	subs    Subscriptions // The subscriptions that exist within this project
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

	// Bring the queues collection uptodate with what the system has in terms
	// of functioning queues
	//
	added, removed := qr.subs.align(known)
	for _, add := range added {
		logger.Trace("added queue", "queue", add, "stack", stack.Trace().TrimRuntime())
	}
	for _, remove := range removed {
		logger.Trace("removed queue", "queue", remove, "stack", stack.Trace().TrimRuntime())
	}
	return nil
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

// producer is used to examine the subscriptions that are available and determine if
// capacity is available to service any of the work that might be waiting
//
func (qr *Queuer) producer(ctx context.Context, rqst chan *SubRequest) {

	logger.Trace("started queue producer")
	defer logger.Trace("stopped queue producer")

	check := time.NewTicker(time.Duration(5 * time.Second))
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
						if len(ranked) != 0 {
							logger.Trace(fmt.Sprintf("processing %d ranked subscriptions %v", len(ranked), ranked))
						} else {
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
					if _, isPresent := backoffs.Get(qr.project + ":" + sub.name); isPresent {
						logger.Trace(fmt.Sprintf("backed off %s:%s", qr.project, sub.name), "stack", stack.Trace().TrimRuntime())
						continue
					}
					// Save the queue that has been waiting the longest into the
					// idle slot that we will be processing on this pass
					idle = append(idle, sub)
				}
			}

			if len(idle) != 0 {

				// Shuffle the queues to pick one at random, fisher yates shuffle introduced in
				// go 1.10, c.f. https://golang.org/pkg/math/rand/#Shuffle
				rand.Shuffle(len(idle), func(i, j int) {
					idle[i], idle[j] = idle[j], idle[i]
				})

				if err := qr.check(ctx, idle[0].name, rqst); err != nil {

					backoffs.Set(qr.project+":"+idle[0].name, true, time.Duration(time.Minute))

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
	if !isPresent {
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

// getMachineResources extracts the current system state in terms of memory etc
// and coverts this into the resource specification used by jobs.  Because resources
// specified by users are not exact quantities the resource is used for the machines
// resources even in the face of some loss of precision
//
func getMachineResources() (rsc *runner.Resource) {

	rsc = &runner.Resource{}

	// For specified queue look for any free slots on existing GPUs is
	// applicable and fill them, or find empty GPUs and groups to fill
	// in with work

	cpus, v := runner.CPUFree()
	rsc.Cpus = uint(cpus)
	rsc.Ram = humanize.Bytes(v)

	rsc.Hdd = humanize.Bytes(runner.GetDiskFree())

	// go runner allows GPU resources at the board level so obtain the total slots across
	// all board form factors and use that as our max
	//
	rsc.Gpus = runner.TotalFreeGPUSlots()
	rsc.GpuMem = humanize.Bytes(runner.LargestFreeGPUMem())

	return rsc
}

// check will first validate a subscription and will add it to the list of subscriptions
// to be processed, which is in turn used by the scheduler later.
//
func (qr *Queuer) check(ctx context.Context, name string, rQ chan *SubRequest) (err kv.Error) {

	// Check to see if anyone is listening for a queue to check by sending a dummy request, and then
	// send the real request if the check message is consumed
	select {
	case rQ <- &SubRequest{}:
	default:
		return kv.NewError("busy consumer, at the 1ˢᵗ stage").With("stack", stack.Trace().TrimRuntime())
	}

	sub, isPresent := qr.subs.subs[name]
	if !isPresent {
		return kv.NewError("subscription not found").With("project", qr.project, "subscription", name).With("stack", stack.Trace().TrimRuntime())
	}

	if sub.rsc != nil {
		if fit, err := sub.rsc.Fit(getMachineResources()); !fit {
			if err != nil {
				return err
			}

			if logger.IsTrace() {
				logger.Trace("no fit", "project", qr.project, "subscription", name, "rsc", sub.rsc, "headroom", getMachineResources(),
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
// This function will block except in the case a fatal issue occurs that prevents it
// from being able to perform the function that it is intended to do
//
func (qr *Queuer) run(ctx context.Context, refreshInterval time.Duration) (err kv.Error) {

	// Start a single unbuffered worker that we have for now to trigger for work
	sendWork := make(chan *SubRequest)
	go qr.consumer(ctx, sendWork)

	// start work producer that looks at subscriptions and then checks the
	// sendWork listener to ensure there is capacity

	go qr.producer(ctx, sendWork)

	refresh := time.Duration(time.Second)

	for {
		select {
		case <-time.After(refresh):
			if err := qr.refresh(); err != nil {
				return err
			}
			// Check for new queues or deleted queues once every few minutes
			refresh = time.Duration(refreshInterval)
		case <-ctx.Done():
			return nil
		}
	}
}

func (qr *Queuer) consumer(ctx context.Context, readyC chan *SubRequest) {

	logger.Debug("started consumer", "project", qr.project)
	defer logger.Debug("stopped consumer", "project", qr.project)

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

// filterWork handles requests to check queues for work.  Before doing the work
// it will however also check to ensure that a backoff time is not in play
// for the queue, if it is then it will simply return
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

	busyQs.Lock()
	_, busy := busyQs.subs[request.project+":"+request.subscription]
	if !busy {
		busyQs.subs[request.project+":"+request.subscription] = true
	}
	busyQs.Unlock()

	if busy {
		logger.Trace(fmt.Sprintf("busy %v", request))
		return
	}
	logger.Trace(fmt.Sprintf("mark as busy %v", request))

	defer func() {
		busyQs.Lock()
		delete(busyQs.subs, request.project+":"+request.subscription)
		busyQs.Unlock()

		logger.Trace(fmt.Sprintf("mark as free %v", request))
	}()

	qr.doWork(ctx, request)
}

// HandleMsg takes a message describing a queued task and handles the request, running and validating it
// in a blocking fashion
//
func HandleMsg(ctx context.Context, qt *runner.QueueTask) (rsc *runner.Resource, consume bool) {

	rsc = nil

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("%#v", r), "stack", stack.Trace().TrimRuntime())
		}
	}()

	// Check for the back off and self destruct if one is seen for this subscription, leave the message for
	// redelivery upto the framework
	//
	// TODO Ack for PubSub Nack for SQS due to SQS supporting dead letter queues
	//
	if _, isPresent := backoffs.Get(qt.Project + ":" + qt.Subscription); isPresent {
		logger.Debug("stopping checking backing off", "project_id", qt.Project, "subscription", qt.Subscription)
		return rsc, false
	}

	logger.Debug("msg processing started", "project_id", qt.Project, "subscription", qt.Subscription)
	defer logger.Debug("msg processing done", "project_id", qt.Project, "subscription", qt.Subscription)

	// allocate the processor and sub the subscription as
	// the group mechanism for work coming down the
	// pipe that is sent to the resource allocation
	// module
	proc, err := newProcessor(ctx, qt.Subscription, qt.Msg, qt.Credentials)
	if err != nil {
		logger.Warn("unable to process msg", "project_id", qt.Project, "subscription", qt.Subscription, "error", err.Error())

		backoffs.Set(qt.Project+":"+qt.Subscription, true, time.Duration(10*time.Second))
		return rsc, true
	}
	defer proc.Close()

	rsc = proc.Request.Experiment.Resource.Clone()

	labels := prometheus.Labels{
		"host":       host,
		"queue_type": "rmq",
		"queue_name": qt.Project + qt.Subscription,
		"project":    proc.Request.Config.Database.ProjectId,
		"experiment": proc.Request.Experiment.Key,
	}

	// Modify the prometheus metrics that track running jobs
	queueRunning.With(labels).Inc()
	defer func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Info("unable to update counters", "recover", fmt.Sprint(r), "stack", stack.Trace().TrimRuntime())
			}
		}()
		queueRunning.With(labels).Dec()
		queueRan.With(labels).Inc()
	}()

	logger.Info("validating experiment", "project_id", proc.Request.Config.Database.ProjectId,
		"experiment_id", proc.Request.Experiment.Key)

	startTime := time.Now()

	// Blocking call to run the entire task and only return on termination due to the context
	// being cancelled or its own error / success
	backoff, ack, err := proc.Process(ctx)
	if err != nil {

		// Do at least a minimal backoff
		if backoff == time.Duration(0) {
			backoff = time.Second
		}

		backoffs.Set(qt.Project+":"+qt.Subscription, true, backoff)

		if !ack {
			logger.Info("retry experiment", "project_id", proc.Request.Config.Database.ProjectId, "experiment_id", proc.Request.Experiment.Key, "error", err.Error())
		} else {
			logger.Warn("dump experiment", "project_id", proc.Request.Config.Database.ProjectId, "experiment_id", proc.Request.Experiment.Key, "error", err.Error())
		}

		return rsc, ack
	}

	logger.Info("completed experiment", "project_id", proc.Request.Config.Database.ProjectId,
		"experiment_id", proc.Request.Experiment.Key, "duration", time.Since(startTime).String(),
		"stack", stack.Trace().TrimRuntime())

	// At this point we could look for a backoff for this queue and set it to a small value as we are about to release resources
	if _, isPresent := backoffs.Get(qt.Project + ":" + qt.Subscription); isPresent {
		backoffs.Set(qt.Project+":"+qt.Subscription, true, time.Second)
	}
	return rsc, ack
}

func (qr *Queuer) doWork(ctx context.Context, request *SubRequest) {

	if _, isPresent := backoffs.Get(request.project + ":" + request.subscription); isPresent {
		logger.Trace(fmt.Sprintf("%v, backed off", request))
		return
	}

	logger.Trace(fmt.Sprintf("started checking %#v", *request))
	defer logger.Trace(fmt.Sprintf("stopped checking for %#v", *request))

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic running studioml script %#v, %s", r, string(debug.Stack())))
		}
	}()

	cCtx, workCancel := context.WithTimeout(context.Background(), qr.timeout)

	go func() {
		logger.Trace(fmt.Sprintf("started queue check %#v", *request))
		defer logger.Trace(fmt.Sprintf("completed queue check for %#v", *request))

		// Spins out a go routine to handle messages, HandleMsg will be invoked
		// by the queue specific implementation in the event that valid work is found
		//
		qt := &runner.QueueTask{
			FQProject:    qr.project,
			Project:      request.project,
			Subscription: request.subscription,
			Handler:      HandleMsg,
		}

		// Establish new context with the timeouts for the queue runner in place.
		// The shadowing is intentional
		//
		defer workCancel()

		cnt, rsc, errGo := qr.tasker.Work(ctx, qt)

		if errGo != nil {
			backoffTime := time.Duration(2 * time.Minute)
			msg := fmt.Sprint(errGo)
			if err, ok := errGo.(kv.Error); ok {
				msg = fmt.Sprint(err)
			}
			logger.Warn(fmt.Sprintf("backing off %v, %v msg receive failed due to %s", backoffTime,
				request, strings.Replace(msg, "\n", "", 0)))
			backoffs.Set(request.project+":"+request.subscription, true, backoffTime)
			return
		}

		// Set the default resource requirements for the next message fetch to that of the most recently
		// seen resource request
		//
		if rsc == nil {
			if cnt > 0 {
				backoffTime := time.Duration(2 * time.Minute)
				logger.Debug(fmt.Sprintf("backing off %v, %v", backoffTime, request))
				backoffs.Set(request.project+":"+request.subscription, true, backoffTime)
			}
			return
		}
		if err := qr.subs.setResources(request.subscription, rsc); err != nil {
			logger.Info(fmt.Sprintf("%s:%s resources not updated due to %s", request.project, request.subscription, err))
		}

	}()

	// While waiting for this check periodically that the queue that
	// was used to send the message still exists, if it does not cancel
	// everything as this is an indication that the work is intended to
	// be stopped in a minute or so
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

			case <-cCtx.Done():
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}
