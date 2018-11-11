package runner

// This file contains the implementation of googles PubSub message queues
// as they are used by studioML

import (
	"flag"
	"regexp"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	pubsubTimeoutOpt = flag.Duration("pubsub-timeout", time.Duration(5*time.Second), "the period of time discrete pubsub operations use for timeouts")
)

type PubSub struct {
	project string
	creds   string
}

func NewPubSub(project string, creds string) (ps *PubSub, err errors.Error) {
	return &PubSub{
		project: project,
		creds:   creds,
	}, nil
}

// Refresh uses a regular expression to obtain matching queues from
// the configured Google pubsub server on gcloud (ps).
//
func (ps *PubSub) Refresh(ctx context.Context, qNameMatch *regexp.Regexp) (known map[string]interface{}, err errors.Error) {

	known = map[string]interface{}{}

	// Intentional shadowing of ctx to layer a new timeout onto a new context
	// that retains the behavior of the original
	ctx, cancel := context.WithTimeout(ctx, *pubsubTimeoutOpt)
	defer cancel()

	client, errGo := pubsub.NewClient(ctx, ps.project, option.WithCredentialsFile(ps.creds))
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer client.Close()

	// Get all of the known subscriptions in the project and make a record of them
	subs := client.Subscriptions(ctx)
	for {
		sub, errGo := subs.Next()
		if errGo == iterator.Done {
			break
		}
		if errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		known[sub.ID()] = true
	}

	return known, nil
}

// Exists will connect to the google pubsub server identified in the receiver, ps, and will
// query it to see if the queue identified by the studio go runner subscription exists
//
func (ps *PubSub) Exists(ctx context.Context, subscription string) (exists bool, err errors.Error) {
	client, errGo := pubsub.NewClient(ctx, ps.project, option.WithCredentialsFile(ps.creds))
	if errGo != nil {
		return true, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("project", ps.project)
	}
	defer client.Close()

	exists, errGo = client.Subscription(subscription).Exists(ctx)
	if errGo != nil {
		return true, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("project", ps.project)
	}
	return exists, nil
}

// Work will connect to the google pubsub server identified in the receiver, ps, and will see if any work
// can be found on the queue identified by the go runner subscription and present work
// to the handler for processing
//
func (ps *PubSub) Work(ctx context.Context, qt *QueueTask) (msgs uint64, resource *Resource, err errors.Error) {

	client, errGo := pubsub.NewClient(ctx, ps.project, option.WithCredentialsFile(ps.creds))
	if errGo != nil {
		return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("project", ps.project)
	}
	defer client.Close()

	sub := client.Subscription(qt.Subscription)
	sub.ReceiveSettings.MaxExtension = time.Duration(12 * time.Hour)

	errGo = sub.Receive(ctx,
		func(ctx context.Context, msg *pubsub.Message) {

			qt.Credentials = ps.creds
			qt.Project = ps.project
			qt.Msg = msg.Data

			if rsc, ack := qt.Handler(ctx, qt); ack {
				msg.Ack()
				resource = rsc
			} else {
				msg.Nack()
			}
			atomic.AddUint64(&msgs, 1)
		})

	if errGo != nil {
		return msgs, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return msgs, resource, nil
}
