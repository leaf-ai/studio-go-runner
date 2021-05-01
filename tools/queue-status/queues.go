// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func GetQueues(ctx context.Context, cfg *Config) (queues Queues, err kv.Error) {
	opts := session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}
	if len(cfg.accessKey) != 0 || len(cfg.secretKey) != 0 {
		if len(cfg.accessKey) == 0 {
			return nil, kv.NewError("secret key specified but access key was not specified").With("stack", stack.Trace().TrimRuntime())
		}
		if len(cfg.secretKey) == 0 {
			return nil, kv.NewError("secret key not specified but access key was specified").With("stack", stack.Trace().TrimRuntime())
		}
		if len(cfg.region) == 0 {
			return nil, kv.NewError("region needs to be supplied when keys are specified").With("stack", stack.Trace().TrimRuntime())
		}
		opts = session.Options{
			Config: aws.Config{
				Credentials: credentials.NewStaticCredentials(cfg.accessKey, cfg.secretKey, ""),
				Region:      &cfg.region,
			},
		}
	}

	sess, errGo := session.NewSessionWithOptions(opts)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	svc := sqs.New(sess)

	getAll := "All"
	getOpts := sqs.GetQueueAttributesInput{
		AttributeNames: []*string{&getAll},
	}
	queues = Queues{}
	errGo = svc.ListQueuesPages(nil,
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
				getOpts.QueueUrl = q
				output, errGo := svc.GetQueueAttributesWithContext(ctx, &getOpts)
				if errGo != nil {
					err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
					return false
				}
				status := QStatus{
					name: name,
				}
				msgs, isPresent := output.Attributes["ApproximateNumberOfMessages"]
				if !isPresent {
					err = kv.NewError("message count unavailable").With("stack", stack.Trace().TrimRuntime())
					return false
				}
				if msgs != nil && len(*msgs) != 0 {
					msgCount, errGo := strconv.Atoi(*msgs)
					if errGo != nil {
						err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
						return false
					}
					status.queued = msgCount

				}
				msgs, isPresent = output.Attributes["ApproximateNumberOfMessagesNotVisible"]
				if !isPresent {
					err = kv.NewError("message count unavailable").With("stack", stack.Trace().TrimRuntime())
					return false
				}
				if msgs != nil && len(*msgs) != 0 {
					msgCount, errGo := strconv.Atoi(*msgs)
					if errGo != nil {
						err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
						return false
					}
					status.working = msgCount

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
