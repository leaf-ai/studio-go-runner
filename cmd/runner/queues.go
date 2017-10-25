package main

// This file contains the implementation of queue query functions along with
// tracking for queues to detect new arrivals and also to detect the
// disappearance of queues
//
// As queues come and go subscriptions are automatically created/accessed so that
// messages have a chance to be noticed

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/davecgh/go-spew/spew"
	"github.com/dustin/go-humanize"

	"github.com/karlmutch/go-cache"
)

var (
	pubsubTimeoutOpt = flag.Duration("pubsub-timeout", time.Duration(5*time.Second), "the period of time discrete pubsub operations use for timeouts")

	// backoffs are a set of values that when they are still alive in the cache the
	// server will not attempt to communicate with the queues they represent.  When the
	// cache entries that represent the queues expire then they are deemed to be ready
	// for more communication.
	//
	// The TTL cache represents the signal to not do something, think of it as a
	// negative signal that has an expiry time.
	//
	// Create a cache with a default expiration time of 1 minute, and which
	// purges expired items every 10 seconds
	//
	backoffs = cache.New(10*time.Second, time.Minute)

	// busyQs is used to indicate when a worker is active for a named queue so
	// that only one is activate at a time
	//
	busyQs = Queues{queues: map[string]*Queue{}}
)

type Queue struct {
	name string           // The subscription name that represents a queue for our purposes
	rsc  *runner.Resource // If known the resources that experiments asked for in this subscription
	cnt  uint             // The number of instances that are running for this queue
}

type Queues struct {
	queues map[string]*Queue // The catalog of all known queues (subscriptions) within the project this server is handling
	sync.Mutex
}

type Queuer struct {
	projectID string
	queues    Queues
}

type queueRequest struct {
	queue string
	opts  option.ClientOption
}

func NewQueuer(projectID string) (qr *Queuer, err error) {
	return &Queuer{
		projectID: projectID,
		queues:    Queues{queues: map[string]*Queue{}},
	}, err
}

func getPubSubCreds() (opts option.ClientOption, err error) {
	val, isPresent := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	if !isPresent {
		return nil, fmt.Errorf(`the environment variable GOOGLE_APPLICATION_CREDENTIALS was not set,
		fix this by creating a service account key using your Web based GCP console and then save the 
		resulting file into a safe location and define an environment variable 
		GOOGLE_APPLICATION_CREDENTIALS to point at this file`)
	}

	return option.WithServiceAccountFile(val), nil
}

func (qr *Queuer) refreshQueues(opts option.ClientOption) (err error) {

	ctx, cancel := context.WithTimeout(context.Background(), *pubsubTimeoutOpt)
	defer cancel()

	client, err := pubsub.NewClient(ctx, qr.projectID, opts)
	if err != nil {
		return err
	}
	defer client.Close()

	// Get all of the known subscriptions in the project and make a record of them
	subs := client.Subscriptions(ctx)
	known := map[string]interface{}{}
	for {
		sub, err := subs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		known[sub.ID()] = true
	}

	// Bring the queues collection uptodate with what the system has in terms
	// of functioning queues
	//
	added, removed := qr.queues.align(known)
	msg := ""
	if 0 != len(added) {
		msg += fmt.Sprintf("added queues %s", strings.Join(added, ", "))
	}
	if 0 != len(removed) {
		msg = strings.Join([]string{msg, fmt.Sprintf("removed queues %s", strings.Join(removed, ", "))}, ", and ")
	}
	if 0 != len(msg) {
		msg = fmt.Sprintf("project %s %s", qr.projectID, msg)
		logger.Info(msg)
		runner.InfoSlack(msg, []string{})
	}
	return nil
}

// align allows the caller to take the extant subscriptions and add or remove them from the list of subscriptions
// we currently have cached
//
func (queues *Queues) align(expected map[string]interface{}) (added []string, removed []string) {
	added = []string{}
	removed = []string{}

	queues.Lock()
	defer queues.Unlock()

	for sub, _ := range expected {
		if _, isPresent := queues.queues[sub]; !isPresent {

			queues.queues[sub] = &Queue{name: sub}
			added = append(added, sub)
		}
	}

	for sub, _ := range queues.queues {
		if _, isPresent := expected[sub]; !isPresent {

			delete(queues.queues, sub)
			removed = append(removed, sub)
		}
	}

	return added, removed
}

// setResources is used to update the resources a queue will generally need for
// its individual work items
//
func (queues *Queues) setResources(queue string, rsc *runner.Resource) (err error) {
	if rsc == nil {
		return fmt.Errorf("clearing the resource spec for queue %s not supported", queue)
	}

	queues.Lock()
	defer queues.Unlock()

	q, isPresent := queues.queues[queue]
	if !isPresent {
		return fmt.Errorf("queue %s was not present", queue)
	}

	q.rsc = rsc

	return nil
}

// shuffleStrings does a fisher-yates shuffle.  This will be introduced in Go 1.10
// as a standard function.  For now we have to do it ourselves. Copied from
// https://gist.github.com/quux00/8258425
//
func shuffleStrings(slc []string) {
	n := len(slc)
	for i := 0; i < n; i++ {
		// choose index uniformly in [i, n-1]
		r := i + rand.Intn(n-i)
		slc[r], slc[i] = slc[i], slc[r]
	}
}

// producer is used to examine the queues that are available and determine if
// capacity is available to service any of the queues
//
func (qr *Queuer) producer(rQ chan *queueRequest, quitC chan bool) {

	logger.Debug("started the queue checking producer")
	defer logger.Debug("stopped the queue checking producer")

	qCheck := time.NewTicker(time.Duration(5 * time.Second))
	defer qCheck.Stop()

	nextQDbg := time.Now()
	lastQs := 0

	for {
		select {
		case <-qCheck.C:

			ranked := qr.rankQueues()

			// Some monitoring logging used to tracking traffic on queues
			if logger.IsTrace() {
				logger.Trace(fmt.Sprintf("processing %d ranked queues %#v", len(ranked), spew.Sdump(ranked)))
			} else {
				if logger.IsDebug() {
					// If either the queue length has changed, or sometime has passed since
					// the last debug log, one minute, print the queue checking state
					if nextQDbg.Before(time.Now()) || lastQs != len(ranked) {
						lastQs = len(ranked)
						nextQDbg = time.Now().Add(10 * time.Minute)
						logger.Debug(fmt.Sprintf("processing %d ranked queues %#v", len(ranked), ranked))
					}
				}
			}

			// track the first queue that has not been checked for the longest period of time that
			// also has no traffic on this node.  This queue will be check but it wont be until the next
			// pass that a new empty or idle queue will be checked.
			idleQueues := make([]string, 0, len(ranked))

			for _, queue := range ranked {
				// IDLE queue processing, that is queues that have no work running
				// against this runner
				if queue.cnt == 0 {
					if _, isPresent := backoffs.Get(queue.name); isPresent {
						continue
					}
					// Save the queue that has been waiting the longest into the
					// idle slot that we will be processing on this pass
					idleQueues = append(idleQueues, queue.name)
				}
			}

			if len(idleQueues) != 0 {

				// Shuffle the queues to pick one at random
				shuffleStrings(idleQueues)

				if err := qr.check(idleQueues[0], rQ, quitC); err != nil {

					backoffs.Set(idleQueues[0], true, time.Duration(time.Minute))

					logger.Warn(fmt.Sprintf("checking %s for work failed due to %s, backoff 1 minute", idleQueues[0], err.Error()))
					break
				}
			}

		case <-quitC:
			return
		}
	}
}

func (qr *Queuer) getResources(queue string) (rsc *runner.Resource) {
	qr.queues.Lock()
	defer qr.queues.Unlock()

	item, isPresent := qr.queues.queues[queue]
	if !isPresent {
		return nil
	}
	return item.rsc.Clone()
}

// Retrieve the queues and count their occupancy, then sort ascending into
// an array
func (qr *Queuer) rankQueues() (ranked []Queue) {
	qr.queues.Lock()
	defer qr.queues.Unlock()

	ranked = make([]Queue, 0, len(qr.queues.queues))
	for _, queue := range qr.queues.queues {
		ranked = append(ranked, *queue)
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

	// go runner allows GPU resources at the board level so obtain the largest single
	// board form factor and use that as our max
	//
	rsc.Gpus = runner.LargestFreeGPUSlots()
	rsc.GpuMem = humanize.Bytes(runner.LargestFreeGPUMem())

	return rsc
}

// check will examine a queue and will add it to the list of queues that it expects
// should be processed, which is in turn used by the scheduler later
//
func (qr *Queuer) check(queueName string, rQ chan *queueRequest, quitC chan bool) (err error) {

	// Check to see if anyone is listening for a queue to check by sending a dummy request and then
	// send the real request
	select {
	case rQ <- &queueRequest{}:
	default:
		return fmt.Errorf("busy queue checking consumer, at the 1ˢᵗ stage")
	}

	queue, isPresent := qr.queues.queues[queueName]
	if !isPresent {
		return fmt.Errorf("queue %s could not be found", queueName)
	}

	if queue.rsc != nil {
		if fit, err := queue.rsc.Fit(getMachineResources()); !fit {
			if err != nil {
				return err
			}

			return fmt.Errorf("queue %s could not be accomodated %#v -> %#v", queueName, queue.rsc, getMachineResources())
		} else {
			if logger.IsTrace() {
				logger.Trace(fmt.Sprintf("queue %s passed capacity check", queueName))
			}
		}
	} else {
		if logger.IsTrace() {
			logger.Trace(fmt.Sprintf("queue %s skipped capacity check", queueName))
		}
	}

	opts, err := getPubSubCreds()
	if err != nil {
		return fmt.Errorf("queue check %s failed to get credentials due to %s", queueName, err.Error())
	}

	select {
	case rQ <- &queueRequest{queue: queueName, opts: opts}:
	case <-time.After(2 * time.Second):
		return fmt.Errorf("busy queue checking consumer, at the 2ⁿᵈ stage")
	}

	// Check resource allocation availability to guide fetching work from queues
	// based upon the project ID we have been given
	/**
	gpus := map[string]runner.GPUTrack{}

	// First if we have gpuSlots and mem then look for free gpus slots for
	// the project and if we dont find project specific slots check if
	// we should be using an unassigned device
	if slots != 0 && gpuMem != 0 {
		// Look at GPU devices to see if we can identify bound queues to
		// cards with capacity and fill those, 1 at a time
		gpus = runner.FindGPUs(queue, slots, mem)
		if len(gpus) == 0 {
			gpus = runner.FindGPUs("", slots, mem)
			if len(gpus) == 0 {
				return nil
			}
		}
	}
	**/
	return nil
}

// run will execute maintenance operations in the back ground for the server looking for new
// or old queues can adding then or removing them as needed
//
// This function will block except in the case a fatal issue occurs that prevents it
// from being able to perform the function that it is intended to do
//
func (qr *Queuer) run(quitC chan bool) (err error) {

	// Start a single unbuffered worker that we have for now to trigger for work
	sendWork := make(chan *queueRequest)
	go qr.consumer(sendWork, quitC)

	// start work producer that looks at queues and then checks the
	// sendWork listener to ensure there is capacity

	go qr.producer(sendWork, quitC)

	cred, err := getPubSubCreds()
	if err != nil {
		return err
	}

	refresh := time.Duration(time.Second)

	for {
		select {
		case <-time.After(refresh):
			if err := qr.refreshQueues(cred); err != nil {
				logger.Warn(err.Error())
			}
			refresh = time.Duration(time.Minute)
		case <-quitC:
			return nil
		}
	}
}

func (qr *Queuer) consumer(readyC chan *queueRequest, quitC chan bool) {

	logger.Debug("started the queue checking consumer")
	defer logger.Debug("stopped the queue checking consumer")

	for {
		select {
		case request := <-readyC:
			// The channel looks to have been closed so stop handling work
			if request == nil {
				return
			}
			// An empty structure will be sent when the sender want to check if
			// the worker is ready for a scheduling request for a queue
			if len(request.queue) == 0 {
				continue
			}
			go qr.filterWork(request, quitC)
		case <-quitC:
			return
		}
	}
}

// filterWork handles requests to check queues for work.  Before doing the work
// it will however also check to ensure that a backoff time is not in play
// for the queue, if it is then it will simply return
//
func (qr *Queuer) filterWork(request *queueRequest, quitC chan bool) {

	if _, isPresent := backoffs.Get(request.queue); isPresent {
		logger.Debug(fmt.Sprintf("queue %s is in a backoff state", request.queue))
		return
	}

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic in filterWork %#v, %s", r, string(debug.Stack())))
		}
	}()

	busy := false

	busyQs.Lock()
	if _, busy = busyQs.queues[request.queue]; !busy {
		busyQs.queues[request.queue] = &Queue{name: request.queue}
		logger.Debug(fmt.Sprintf("queue %s marked as busy", request.queue))
	}
	busyQs.Unlock()

	if busy {
		logger.Trace(fmt.Sprintf("queue %s busy", request.queue))
		return
	}

	defer func() {
		busyQs.Lock()
		defer busyQs.Unlock()

		delete(busyQs.queues, request.queue)
		logger.Debug(fmt.Sprintf("cleared queue %s busy", request.queue))
	}()

	qr.doWork(request, quitC)

}

func (qr *Queuer) doWork(request *queueRequest, quitC chan bool) {

	logger.Debug(fmt.Sprintf("started queue check %#v", *request))
	defer logger.Debug(fmt.Sprintf("stopped queue check for %#v", *request))

	defer func() {
		if r := recover(); r != nil {
			logger.Warn(fmt.Sprintf("panic running studioml script %#v, %s", r, string(debug.Stack())))
		}
	}()

	cCtx, cCancel := context.WithTimeout(context.Background(), *pubsubTimeoutOpt)
	defer cCancel()

	client, err := pubsub.NewClient(cCtx, qr.projectID, request.opts)
	if err != nil {
		logger.Warn(fmt.Sprintf("failed starting queue listener %s due to %v", request.queue, err))
		return
	}
	defer client.Close()

	rCtx, rCancel := context.WithCancel(context.Background())
	defer func() {
		defer func() {
			recover()
		}()
		rCancel()
	}()

	sub := client.Subscription(request.queue)
	sub.ReceiveSettings.MaxExtension = time.Duration(12 * time.Hour)

	logger.Debug(fmt.Sprintf("waiting queue request %#v", *request))
	defer logger.Debug(fmt.Sprintf("stopped queue request for %#v", *request))

	err = sub.Receive(rCtx,
		func(ctx context.Context, msg *pubsub.Message) {

			defer func() {
				if r := recover(); r != nil {
					logger.Warn(fmt.Sprintf("%#v", r))
				}
			}()
			logger.Debug(fmt.Sprintf("msg processing started on queue %s", request.queue))
			defer logger.Debug(fmt.Sprintf("msg processing completed on queue %s", request.queue))

			// Check for the back off and self destruct if one is seen for this queue, leave the message for
			// redelivery upto the framework
			if _, isPresent := backoffs.Get(request.queue); isPresent {
				defer rCancel()
				logger.Info(fmt.Sprintf("stopping queue %s checking backing off", request.queue))
				msg.Nack()
				return
			}

			// allocate the processor and sub the subscription as
			// the group mechanisim for work comming down the
			// pipe that is sent to the resource allocation
			// module
			proc, err := newProcessor(request.queue, msg, quitC)
			if err != nil {
				defer rCancel()
				logger.Warn(fmt.Sprintf("unable to process msg from queue %s due to %s", request.queue, err))
				msg.Nack()
				return
			}
			defer proc.Close()

			// Set the default resource requirements for the next message fetch to that of the most recently
			// seen resource request
			//
			if errGo := qr.queues.setResources(request.queue, proc.Request.Experiment.Resource.Clone()); errGo != nil {
				logger.Info(fmt.Sprintf("queue %s resources not updated due to %s", request.queue, errGo.Error()))
			}

			logger.Info(fmt.Sprintf("started queue %s experiment %s", request.queue, proc.Request.Experiment.Key))

			if backoff, ack, err := proc.Process(msg); err != nil {

				if ack {
					msg.Nack()
					txt := fmt.Sprintf("retry queue %s experiment %s, backing off for %s", request.queue, proc.Request.Experiment.Key, backoff)
					runner.InfoSlack(txt, []string{})
					logger.Info(txt)
				} else {
					msg.Ack()
					txt := fmt.Sprintf("dump queue %s experiment %s, backing off for %s", request.queue, proc.Request.Experiment.Key, backoff)

					runner.WarningSlack(txt, []string{})
					logger.Warn(txt)
				}
				logger.Warn(err.Error())

				defer rCancel()

				backoffs.Set(request.queue, true, backoff)

				return
			}

			msg.Ack()
			logger.Info(fmt.Sprintf("acked queue %s experiment %s", request.queue, proc.Request.Experiment.Key))

			// At this point we could look for a backoff for this queue and set it to a small value as we are about to release resources
			if _, isPresent := backoffs.Get(request.queue); isPresent {
				backoffs.Set(request.queue, true, time.Second)
			}
		})

	select {
	case <-cCtx.Done():
		break
	case <-quitC:
		rCancel()
	}

	if err != context.Canceled && err != nil {
		logger.Warn(fmt.Sprintf("queue %s msg receive failed due to %s", request.queue, err.Error()))
	}
}
