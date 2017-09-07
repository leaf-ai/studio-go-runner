package main

// This file contains the implementation of queue query functions along with
// tracking for queues to detect new arrivals and also to detect the
// disappearance of queues
//
// As queues come and go subscriptions are automatically created/accessed so that
// messages have a chance to be noticed

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/SentientTechnologies/studio-go-runner"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"golang.org/x/net/context"
)

type Queue struct {
	name string           // The subscription name that represents a queue for our purposes
	rsc  *runner.Resource // If known the resources that experiments asked for in this subscription
	cnt  uint             // The number of instances that are running for this queue
	wait time.Time        // If specified the time when this queue check should be deplayed until, if high enough can be used to disable the queue
}

type Queues struct {
	queues map[string]Queue // The catalog of all known queues (subscriptions) within the project this server is handling
	sync.Mutex
}

type Queuer struct {
	projectID string
}

type queueRequest struct {
	queue string
	opts  option.ClientOption
}

var (
	queues = Queues{queues: map[string]Queue{}}
)

func NewQueuer(projectID string) (qr *Queuer, err error) {
	return &Queuer{projectID: projectID}, err
}

func getPubSubCreds() (opts option.ClientOption, err error) {
	ps := &runner.PubSub{}
	return ps.GetCred()
}

func (qr *Queuer) refreshQueues(opts option.ClientOption) (err error) {

	ctx := context.Background()

	client, err := pubsub.NewClient(ctx, qr.projectID, opts)
	if err != nil {
		return err
	}
	defer client.Close()

	// Get all of the known subscriptions in the project and make a record of them
	subs := client.Subscriptions(ctx)
	known := map[string]bool{}
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

	// Now take the extant subscriptions and add or remove them from the list of subscriptions
	// we currently know about
	//
	queues.Lock()
	defer queues.Unlock()
	for sub, _ := range known {
		if _, isPresent := queues.queues[sub]; !isPresent {
			logger.Info(fmt.Sprintf("queue added %s", sub))
			queues.queues[sub] = Queue{name: sub}
		}
	}

	for sub, _ := range queues.queues {
		if _, isPresent := known[sub]; !isPresent {
			delete(queues.queues, sub)
			logger.Info(fmt.Sprintf("queue discarded %s", sub))
		}
	}

	return nil
}

// producer is used to examine the queues that are available and determine if
// capacity is available to service any of the queues
//
func (qr *Queuer) producer(rQ chan *queueRequest, quitC chan bool) {
	qCheck := time.Duration(5 * time.Second)

	for {
		select {
		case <-time.After(qCheck):

			for _, queue := range qr.rankQueues() {
				if err := qr.check(queue, rQ, quitC); err != nil {
					logger.Warn(fmt.Sprintf("checking for work failed due to %s", err.Error()))
					break
				}
			}

		case <-quitC:
			return
		}
	}
}

func (qr *Queuer) getResources(queue string) (rsc *runner.Resource) {
	queues.Lock()
	defer queues.Unlock()

	item, isPresent := queues.queues[queue]
	if !isPresent {
		return nil
	}
	return item.rsc.Clone()
}

// Retrieve the queues and count their occupancy, then sort ascending into
// an array
func (*Queuer) rankQueues() (ranked []Queue) {
	queues.Lock()
	defer queues.Unlock()

	ranked = make([]Queue, 0, len(queues.queues))
	for _, queue := range queues.queues {
		ranked = append(ranked, queue)
	}

	// sort the queues by their frequency of work, not their occupany of resources
	// so this is approximate but good enough for now
	//
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].cnt < ranked[j].cnt })

	return ranked
}

func (qr *Queuer) check(queue Queue, rQ chan *queueRequest, quitC chan bool) (err error) {

	// For specified queue look for any free slots on existing GPUs is
	// applicable and fill them, or find empty GPUs and groups to fill
	// in with work

	freeCores, freeMem := runner.GetCPUFree()
	freeDisk := runner.GetDiskFree()

	cores := uint(0)
	mem := uint64(0)

	// Check to ensure that base system resources are good enough
	if freeCores < cores || freeMem < mem {
		return nil
	}

	disk := uint64(0)
	if disk < freeDisk {
		return nil
	}

	// Check resource allocation availability to guide fetching work from queues
	// based upon the project ID we have been given
	gpus := map[string]runner.GPUTrack{}

	slots := uint(0)
	gpuMem := uint64(0)

	// First if we have gpuSlots and mem then look for free gpus slots for
	// the project and if we dont find project specific slots check if
	// we should be using an unassigned device
	if slots != 0 && gpuMem != 0 {
		// Look at GPU devices to see if we can identify bound queues to
		// cards with capacity and fill those, 1 at a time
		gpus = runner.FindGPUs(queue.name, slots, mem)
		if len(gpus) == 0 {
			gpus = runner.FindGPUs("", slots, mem)
			if len(gpus) == 0 {
				return nil
			}
		}
	}

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

	for {
		select {
		case request := <-readyC:
			if request == nil {
				return
			}
			qr.doWork(request, stopC)
		case <-stopC:
			return
		}
	}
}

func (qr *Queuer) doWork(request *queueRequest, stopC chan bool) {

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	client, err := pubsub.NewClient(ctx, qr.projectID, request.opts)
	if err != nil {
		logger.Fatal(fmt.Sprintf("could not start the pubsub listener due to %v", err))
	}
	defer cancel()

	defer client.Close()

	sub := client.Subscription(request.queue)

	err = sub.Receive(ctx,
		func(ctx context.Context, msg *pubsub.Message) {
			// allocate the processor and sub the subscription as
			// the group mechanisim for work comming down the
			// pipe that is sent to the resource allocation
			// module
			proc, err := newProcessor(request.queue)
			if err != nil {
				logger.Warn("unable to create new processor")
				msg.Nack()
				return
			}
			defer proc.Close()
			if _, err := proc.ProcessMsg(msg); err == nil {
				msg.Ack()
			}
		})
}
