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
)

var (
	pubsubTimeoutOpt = flag.Duration("pubsub-timeout", time.Duration(5*time.Second), "the period of time discrete pubsub operations use for timeouts")
)

type Queue struct {
	name string           // The subscription name that represents a queue for our purposes
	rsc  *runner.Resource // If known the resources that experiments asked for in this subscription
	cnt  uint             // The number of instances that are running for this queue
	wait time.Time        // If specified the time when this queue check should be deplayed until, if high enough can be used to disable the queue
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
	ps := &runner.PubSub{}
	return ps.GetCred()
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
	qr.queues.align(known)

	return nil
}

// align allows the caller to take the extant subscriptions and add or remove them from the list of subscriptions
// we currently have cached
//
func (queues *Queues) align(expected map[string]interface{}) {
	queues.Lock()
	defer queues.Unlock()

	for sub, _ := range expected {
		if _, isPresent := queues.queues[sub]; !isPresent {
			logger.Info(fmt.Sprintf("queue added %s", sub))
			queues.queues[sub] = &Queue{name: sub}
		}
	}

	for sub, _ := range queues.queues {
		if _, isPresent := expected[sub]; !isPresent {
			delete(queues.queues, sub)
			logger.Info(fmt.Sprintf("queue discarded %s", sub))
		}
	}
}

// setWait is used to update the next time a queue will be scheduled for checking for work
//
func (queues *Queues) setWait(queue string, newWait time.Time) (err error) {

	queues.Lock()
	defer queues.Unlock()

	q, isPresent := queues.queues[queue]
	if !isPresent {
		return fmt.Errorf("queue %s was not present", queue)
	}
	q.wait = newWait
	return nil
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

// producer is used to examine the queues that are available and determine if
// capacity is available to service any of the queues
//
func (qr *Queuer) producer(rQ chan *queueRequest, quitC chan bool) {

	logger.Debug("started the queue checking producer")
	defer logger.Debug("stopped the queue checking producer")

	qCheck := time.Duration(5 * time.Second)

	nextQDbg := time.Now()
	lastQs := 0

	for {
		select {
		case <-time.After(qCheck):

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
						logger.Debug(fmt.Sprintf("processing %d ranked queues %#v", len(ranked), strings.Replace(spew.Sdump(ranked), "\n", "", -1)))
					}
				}
			}

			// track the first queue that has not been checked for the longest period of time that
			// also has no traffic on this node.  This queue will be check but it wont be until the next
			// pass that a new empty or idle queue will be checked.
			idleQueue := ""
			idleWait := time.Now()

			for _, queue := range ranked {
				// IDLE queue processing, that is queues that have no work running
				// against this runner
				if queue.cnt == 0 {
					// Save the queue that has been waiting the longest into the
					// idle slot that we will be processing on this pass
					if queue.wait.Before(idleWait) {
						idleQueue = queue.name
						idleWait = queue.wait
					}
				}
			}

			if len(idleQueue) != 0 {
				if err := qr.queues.setWait(idleQueue, time.Now().Add(time.Minute)); err != nil {
					logger.Warn(fmt.Sprintf("checking queue %s abandoned due to %s", idleQueue, err.Error()))
					break
				}

				if err := qr.check(idleQueue, rQ, quitC); err != nil {
					logger.Warn(fmt.Sprintf("checking %s for work failed due to %s", idleQueue, err.Error()))
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

func (qr *Queuer) check(queueName string, rQ chan *queueRequest, quitC chan bool) (err error) {

	opts, err := getPubSubCreds()
	if err != nil {
		return fmt.Errorf("queue check %s failed to get credentials due to %s", queueName, err.Error())
	}

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
		if !queue.rsc.Fit(getMachineResources()) {
			return fmt.Errorf("queue %s could not be accomodated\n%s\n%s", queueName, spew.Sdump(queue.rsc), spew.Sdump(getMachineResources()))
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
	go qr.runWork(sendWork, quitC)

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

func (qr *Queuer) runWork(readyC chan *queueRequest, stopC chan bool) {
	logger.Debug("started the queue checking consumer")
	defer logger.Debug("stopped the queue checking consumer")

	for {
		select {
		case request := <-readyC:
			// The channel looks to have been close so stop handling work
			if request == nil {
				return
			}
			// An empty structure will be sent when the sender want to check if
			// the worker is ready for a scheduling request for a queue
			if len(request.queue) == 0 {
				continue
			}
			go qr.doWork(request, stopC)
		case <-stopC:
			return
		}
	}
}

func (qr *Queuer) doWork(request *queueRequest, stopC chan bool) {

	logger.Trace(fmt.Sprintf("started queue check %#v", *request))
	defer logger.Trace(fmt.Sprintf("stopped queue check for %#v", *request))

	ctx, cancel := context.WithTimeout(context.Background(), *pubsubTimeoutOpt)
	defer cancel()

	client, err := pubsub.NewClient(ctx, qr.projectID, request.opts)
	if err != nil {
		logger.Warn(fmt.Sprintf("failed starting queue listener %s due to %v", request.queue, err))
		return
	}

	defer client.Close()

	err = client.Subscription(request.queue).Receive(ctx,
		func(ctx context.Context, msg *pubsub.Message) {
			logger.Debug(fmt.Sprintf("msg processing started on queue %s", request.queue))
			defer logger.Debug(fmt.Sprintf("msg processing completed on queue %s", request.queue))
			// allocate the processor and sub the subscription as
			// the group mechanisim for work comming down the
			// pipe that is sent to the resource allocation
			// module
			proc, err := newProcessor(request.queue, msg)
			if err != nil {
				logger.Warn(fmt.Sprintf("unable to process msg from queue %s due to %s", request.queue, err.Error()))
				msg.Nack()
				return
			}
			defer proc.Close()

			logger.Info(fmt.Sprintf("started queue %s experiment %s", request.queue, proc.Request.Config.Database.ProjectId))
			defer logger.Info(fmt.Sprintf("stopped queue %s experiment %s", request.queue, proc.Request.Config.Database.ProjectId))

			// Set the default resource requirements for the next message fetch to that of the most recently
			// seen resource request
			//
			if err = qr.queues.setResources(request.queue, proc.Request.Config.Resource.Clone()); err != nil {
				logger.Info(fmt.Sprintf("queue %s resources not updated due to %s", request.queue, err.Error()))
			}

			if _, err := proc.Process(msg); err == nil {
				msg.Ack()
				// Have gotten work from the queue take the resources the work
				// requires and save them in our queue cache so that selection
				// of new work can be sensitive to the default resources requested
				// by the queue.
			} else {
				logger.Info(fmt.Sprintf("queue %s work dropped due to %s", request.queue, err.Error()))
			}
		})

	if err != nil {
		logger.Warn(fmt.Sprintf("queue %s msg receive failed due to %s", request.queue, err.Error()))
	}
}
