// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/leaf-ai/studio-go-runner/internal/request"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func GetQueues(ctx context.Context, cfg *Config) (queues Queues, err kv.Error) {
	sess, err := NewSession(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return listQueues(ctx, cfg, sess)
}

func listQueues(ctx context.Context, cfg *Config, sess *session.Session) (queues Queues, err kv.Error) {
	svc := sqs.New(sess)

	getAll := "All"
	getOpts := sqs.GetQueueAttributesInput{
		AttributeNames: []*string{&getAll},
	}

	queues = Queues{}
	errGo := svc.ListQueuesPages(nil,
		func(page *sqs.ListQueuesOutput, lastPage bool) bool {

			for _, q := range page.QueueUrls {
				// If a queue regular expression was supplied use it to select the desired queues
				// and if not output everything
				// Extract the last item in the URL path which is the effective
				// queue name and only use that in the matching
				path := strings.Split(*q, "/")
				name := path[len(path)-1]
				if cfg.queue != nil {
					if !cfg.queue.Match([]byte(name)) {
						continue
					}
				}
				status := QStatus{
					name:     name,
					Resource: nil,
				}

				if err = qMetrics(ctx, svc, &getOpts, q, &status); err != nil {
					return false
				}

				// Examine the first message if available for information
				// as to how much hardware resource is needed for this
				// queue
				if err = qResources(ctx, cfg, svc, *q, &status); err != nil {
					return false
				}

				// If hardware resources are known then populate AWS information
				// about the machines that will be neded to process the work using
				// the current region etc
				if status.Resource != nil {
					costs, err := ec2Instances(ctx, cfg, sess, &status)
					if err != nil {
						return false
					}
					status.Instances = costs
				}

				queues[status.name] = status
			}
			return true
		})
	if err != nil {
		return nil, err
	}
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return queues, nil
}

// qMetrics retrieves message counts and other information for an SQS queue
func qMetrics(ctx context.Context, svc *sqs.SQS, getOpts *sqs.GetQueueAttributesInput, q *string, status *QStatus) (err kv.Error) {
	getOpts.QueueUrl = q
	output, errGo := svc.GetQueueAttributesWithContext(ctx, getOpts)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	// Get the general queue metrics of waiting and working messages
	msgs, isPresent := output.Attributes["ApproximateNumberOfMessages"]
	if !isPresent {
		return kv.NewError("message count unavailable").With("stack", stack.Trace().TrimRuntime())
	}
	if msgs != nil && len(*msgs) != 0 {
		visible, errGo := strconv.Atoi(*msgs)
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		status.Ready = visible
	}
	msgs, isPresent = output.Attributes["ApproximateNumberOfMessagesNotVisible"]
	if !isPresent {
		return kv.NewError("message count unavailable").With("stack", stack.Trace().TrimRuntime())
	}
	if msgs != nil && len(*msgs) != 0 {
		msgCount, errGo := strconv.Atoi(*msgs)
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		status.NotVisible = msgCount

	}
	return nil
}

// qResources extgracts a single message from the queue and uses it to discover what resources
// are needed for the queue
func qResources(ctx context.Context, cfg *Config, svc *sqs.SQS, q string, status *QStatus) (err kv.Error) {
	if status.Ready != 0 {
		one := int64(1) // We need this so that we can use pointers in the option structure
		opt := sqs.ReceiveMessageInput{
			QueueUrl:            &q,
			MaxNumberOfMessages: &one,
			VisibilityTimeout:   &one,
			WaitTimeSeconds:     &one,
		}
		msgs, errGo := svc.ReceiveMessage(&opt)
		if errGo != nil {
			return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		if len(msgs.Messages) > 0 {
			for _, msg := range msgs.Messages {
				if msg.Body != nil {
					rqst, err := request.UnmarshalRequest([]byte(*msg.Body))
					if err != nil {
						return err
					}
					status.Resource = &rqst.Experiment.Resource
					return nil
				}
			}
		}
	}
	return nil
}
