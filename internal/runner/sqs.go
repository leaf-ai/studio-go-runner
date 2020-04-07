// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of AWS SQS message queues
// as they are used by studioML

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/davecgh/go-spew/spew"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
)

// SQS encapsulates an AWS based SQS queue and associated it with a project
//
type SQS struct {
	project string
	creds   *AWSCred
}

// NewSQS creates an SQS data structure using set set of credentials (creds) for
// an sqs queue (sqs)
//
func NewSQS(project string, creds string) (sqs *SQS, err kv.Error) {
	// Use the creds directory to locate all of the credentials for AWS within
	// a hierarchy of directories

	awsCreds, err := AWSExtractCreds(strings.Split(creds, ","))
	if err != nil {
		return nil, err
	}

	return &SQS{
		project: project,
		creds:   awsCreds,
	}, nil
}

func (sq *SQS) listQueues(qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (queues *sqs.ListQueuesOutput, err kv.Error) {

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:                        aws.String(sq.creds.Region),
			Credentials:                   sq.creds.Creds,
			CredentialsChainVerboseErrors: aws.Bool(true),
		},
		Profile: "default",
	})

	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)

	ctx, cancel := context.WithTimeout(context.Background(), *sqsTimeoutOpt)
	defer cancel()

	listParam := &sqs.ListQueuesInput{}

	qs, errGo := svc.ListQueuesWithContext(ctx, listParam)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	fmt.Println(spew.Sdump(qs))
	queues = &sqs.ListQueuesOutput{
		QueueUrls: []*string{},
	}

	for _, qURL := range qs.QueueUrls {
		if qURL == nil {
			continue
		}
		fullURL, errGo := url.Parse(*qURL)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
		}
		paths := strings.Split(fullURL.Path, "/")
		if qNameMismatch != nil {
			if qNameMismatch.MatchString(paths[len(paths)-1]) {
				fmt.Println("dropped", paths[len(paths)-1], qNameMismatch.String())
				continue
			}
		}
		if qNameMatch != nil {
			if !qNameMatch.MatchString(paths[len(paths)-1]) {
				fmt.Println("ignored", paths[len(paths)-1], qNameMatch.String())
				continue
			}
		}
		queues.QueueUrls = append(queues.QueueUrls, qURL)
	}
	fmt.Println(spew.Sdump(queues))
	return queues, nil
}

func (sq *SQS) refresh(qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (known []string, err kv.Error) {

	known = []string{}

	result, err := sq.listQueues(qNameMatch, qNameMismatch)
	if err != nil {
		return known, err
	}

	// As these are pointers, printing them out directly would not be useful.
	for _, url := range result.QueueUrls {
		// Avoid dereferencing a nil pointer.
		if url == nil {
			continue
		}
		known = append(known, *url)
	}
	return known, nil
}

// Refresh uses a regular expression to obtain matching queues from
// the configured SQS server on AWS (sqs).
//
func (sq *SQS) Refresh(ctx context.Context, qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (known map[string]interface{}, err kv.Error) {

	found, err := sq.refresh(qNameMatch, qNameMismatch)
	if err != nil {
		return known, err
	}

	known = make(map[string]interface{}, len(found))
	for _, url := range found {
		known[fmt.Sprintf("%s:%s", sq.creds.Region, url)] = sq.creds
	}

	return known, nil
}

// Exists tests for the presence of a subscription, typically a queue name
// on the configured sqs server.
//
func (sq *SQS) Exists(ctx context.Context, subscription string) (exists bool, err kv.Error) {

	queues, err := sq.listQueues(nil, nil)
	if err != nil {
		return true, err
	}

	for _, q := range queues.QueueUrls {
		if q != nil {
			if strings.HasSuffix(subscription, *q) {
				return true, nil
			}
		}
	}
	return false, nil
}

// Work is invoked by the queue handling software within the runner to get the
// specific queue implementation to process potential work that could be
// waiting inside the queue.
func (sq *SQS) Work(ctx context.Context, qt *QueueTask) (msgProcessed bool, resource *Resource, err kv.Error) {

	regionUrl := strings.SplitN(qt.Subscription, ":", 2)
	url := regionUrl[1]

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:                        aws.String(sq.creds.Region),
			Credentials:                   sq.creds.Creds,
			CredentialsChainVerboseErrors: aws.Bool(true),
		},
		Profile: "default",
	})

	if errGo != nil {
		return false, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)

	defer func() {
		defer func() {
			_ = recover()
		}()
	}()

	visTimeout := int64(30)
	waitTimeout := int64(5)
	msgs, errGo := svc.ReceiveMessageWithContext(ctx,
		&sqs.ReceiveMessageInput{
			QueueUrl:          &url,
			VisibilityTimeout: &visTimeout,
			WaitTimeSeconds:   &waitTimeout,
		})
	if errGo != nil {
		return false, nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}
	if len(msgs.Messages) == 0 {
		return false, nil, nil
	}

	// Make sure that the main ctx has not been Done with before continuing
	select {
	case <-ctx.Done():
		return false, nil, kv.NewError("queue worker cancel received").With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	default:
	}

	// Start a visbility timeout extender that runs until the work is done
	// Changing the timeout restarts the timer on the SQS side, for more information
	// see http://docs.aws.amazon.com/AWSSimpleQueueService/latest/SQSDeveloperGuide/sqs-visibility-timeout.html
	//
	quitC := make(chan struct{})
	go func() {
		timeout := time.Duration(int(visTimeout / 2))
		for {
			select {
			case <-time.After(timeout * time.Second):
				svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueUrl:          &url,
					ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
					VisibilityTimeout: &visTimeout,
				})
			case <-quitC:
				return
			}
		}
	}()

	qt.Project = sq.project
	qt.Subscription = url
	qt.Msg = []byte(*msgs.Messages[0].Body)

	rsc, ack, err := qt.Handler(ctx, qt)
	close(quitC)

	if ack {
		// Delete the message
		svc.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      &url,
			ReceiptHandle: msgs.Messages[0].ReceiptHandle,
		})
		resource = rsc
	} else {
		// Set visibility timeout to 0, in otherwords Nack the message
		visTimeout = 0
		svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
			QueueUrl:          &url,
			ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
			VisibilityTimeout: &visTimeout,
		})
	}

	return true, resource, err
}
