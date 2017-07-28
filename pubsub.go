package runner

// This module contains the code that interfaces with the google pubsub system and
// authentication used by google

import (
	"fmt"
	"os"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"golang.org/x/net/context"
)

var (
	StorageBucket     *storage.BucketHandle
	StorageBucketName string

	PubsubClient *pubsub.Client
)

func getCred() (opts option.ClientOption, err error) {
	val, isPresent := os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	if !isPresent {
		return nil, fmt.Errorf(`the environment variable GOOGLE_APPLICATION_CREDENTIALS was not set,
		fix this by creating a service account key using your Web based GCP console and then save the 
		resulting file into a safe location and define an environment variable 
		GOOGLE_APPLICATION_CREDENTIALS to point at this file`)
	}

	return option.WithServiceAccountFile(val), nil
}

// PubSub encapsulate a Google PubSub msg queue using channels as orchestration
//
type PubSub struct {
	client *pubsub.Client
	topic  *pubsub.Topic
	sub    *pubsub.Subscription
	MsgC   chan *pubsub.Message
	ErrorC chan error
	sync.Mutex
}

// NewPubSub is used to initialize a new PubSub subscriber and listener
//
func NewPubSub(ctx context.Context, projectID string, topicID string, subscriptionID string) (ps *PubSub, err error) {

	ps = &PubSub{
		// No buffering on the msg channel so that Nacks can be done based upon
		// an internal timeout back to the pubsub Q
		MsgC: make(chan *pubsub.Message, 0),
		// ErrorC is used by asynchronous go functions to post any errors they encounter
		ErrorC: make(chan error, 1),
	}

	cred, err := getCred()
	if err != nil {
		return nil, err
	}

	if ps.client, err = pubsub.NewClient(ctx, projectID, cred); err != nil {
		return nil, err
	}

	ps.topic, err = ps.client.CreateTopic(ctx, projectID)
	if err != nil {
		if grpc.Code(err) != codes.AlreadyExists {
			return nil, err
		}
	}

	ps.sub, err = ps.client.CreateSubscription(ctx, subscriptionID, pubsub.SubscriptionConfig{Topic: ps.topic})
	if err != nil {
		if grpc.Code(err) != codes.AlreadyExists {
			return nil, err
		}
	}
	return ps, nil
}

// Close is used to release any resources associated with the google pubsub listeners, including pending
// operations that await completion
//
func (ps *PubSub) Close() (err error) {
	if ps == nil {
		return fmt.Errorf("nil pubsub supplied")
	}

	ps.Lock()
	defer ps.Unlock()

	if ps.client == nil {
		return nil
	}

	ps.topic.Stop()

	ps.client.Close()
	ps.client = nil

	close(ps.MsgC)
	close(ps.ErrorC)

	return nil
}

// run is a blocking function that accepts messages from pubsub and pushes them into
// a go channel for processing, clients should ack messages explicitly otherwise
// they will be resent by pubsub to other subscribers.
//
func (ps *PubSub) run(ctx context.Context) (err error) {
	for {
		err = ps.sub.Receive(ctx,
			func(ctx context.Context, msg *pubsub.Message) {
				select {
				case ps.MsgC <- msg:
					msg.Ack()
				case <-time.After(time.Second):
					msg.Nack()
				}
			})
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case ps.ErrorC <- err:
				continue
			case <-time.After(time.Second):
				// The error has no listeners, we have nothing that
				// can be done after this point other than return
				return err
			}
		}
	}
}

func (ps *PubSub) Start(ctx context.Context) (err error) {

	if ps.client == nil {
		return fmt.Errorf("pubsub not activated, or has been closed")
	}

	go func() {
		ps.run(ctx)
	}()

	return nil
}
