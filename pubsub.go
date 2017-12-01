package runner

// This file contains the implementation of googles PubSub message queues
// as they are used by studioML

import (
	"flag"
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

type PubSub struct{}

func (*PubSub) Refresh(project string, credentials string, timeout time.Duration) (known map[string]interface{}, err errors.Error) {

	known = map[string]interface{}{}

	ctx, cancel := context.WithTimeout(context.Background(), *pubsubTimeoutOpt)
	defer cancel()

	client, errGo := pubsub.NewClient(ctx, project, option.WithCredentialsFile(credentials))
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

func (*PubSub) Work(ctx context.Context, project string, subscription string, credentials string, handler MsgHandler) (resource *Resource, err errors.Error) {

	client, errGo := pubsub.NewClient(ctx, project, option.WithCredentialsFile(credentials))
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("project", project)
	}
	defer client.Close()

	sub := client.Subscription(subscription)
	sub.ReceiveSettings.MaxExtension = time.Duration(12 * time.Hour)

	errGo = sub.Receive(ctx,
		func(ctx context.Context, msg *pubsub.Message) {

			if rsc, ack := handler(ctx, project, subscription, credentials, msg.Data); ack {
				msg.Ack()
				resource = rsc
			} else {
				msg.Nack()
			}
		})

	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return resource, nil
}
