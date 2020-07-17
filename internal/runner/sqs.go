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
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
)

// SQS encapsulates an AWS based SQS queue and associated it with a project
//
type SQS struct {
	project string   // Fully qualified SQS queue reference
	creds   *AWSCred // AWS credentials for access the queue
	wrapper *Wrapper // Decryption information for messages with encrypted payloads
}

// NewSQS creates an SQS data structure using set set of credentials (creds) for
// an sqs queue (sqs)
//
func NewSQS(project string, creds string, wrapper *Wrapper) (sqs *SQS, err kv.Error) {
	// Use the creds directory to locate all of the credentials for AWS within
	// a hierarchy of directories

	awsCreds, err := AWSExtractCreds(strings.Split(creds, ","))
	if err != nil {
		return nil, err
	}

	return &SQS{
		project: project,
		creds:   awsCreds,
		wrapper: wrapper,
	}, nil
}

// GetSQSProjects can be used to get a list of the SQS servers and the main URLs that are accessible to them
func GetSQSProjects(credFiles []string) (urls map[string]struct{}, err kv.Error) {

	sqs, err := NewSQS("aws_probe", strings.Join(credFiles, ","), nil)
	if err != nil {
		return urls, err
	}
	found, err := sqs.refresh(nil, nil)
	if err != nil {
		return urls, kv.Wrap(err, "failed to refresh sqs").With("stack", stack.Trace().TrimRuntime())
	}

	urls = make(map[string]struct{}, len(found))
	for _, urlStr := range found {
		qURL, err := url.Parse(urlStr)
		if err != nil {
			continue
		}
		segments := strings.Split(qURL.Path, "/")
		qURL.Path = strings.Join(segments[:len(segments)-1], "/")
		urls[qURL.String()] = struct{}{}
	}

	return urls, nil
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
	for _, urlStr := range found {
		qURL, err := url.Parse(urlStr)
		if err != nil {
			continue
		}
		segments := strings.Split(qURL.Path, "/")
		known[sq.creds.Region+":"+segments[len(segments)-1]] = sq.creds
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
	url := sq.project + "/" + regionUrl[1]

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
			if r := recover(); r != nil {
				fmt.Printf("panic in producer %#v, %s\n", r, string(debug.Stack()))
			}
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
		return false, nil, kv.Wrap(errGo).With("credentials", sq.creds, "url", url).With("stack", stack.Trace().TrimRuntime())
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
				if _, err := svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueUrl:          &url,
					ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
					VisibilityTimeout: &visTimeout,
				}); err != nil {
					// Once the 1/2 way mark is reached continue to try to change the
					// visibility at decreasing intervals until we finish the job
					if timeout.Seconds() > 5.0 {
						timeout = time.Duration(timeout / 2)
					}
				}
			case <-quitC:
				return
			}
		}
	}()

	qt.Msg = nil
	qt.Msg = []byte(*msgs.Messages[0].Body)

	items := strings.Split(url, "/")
	qt.ShortQName = items[len(items)-1]

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

// HasWork will look at the SQS queue to see if there is any pending work.  The function
// is called in an attempt to see if there is any point in processing new work without a
// lot of overhead.  In the case of SQS at the moment we always assume there is work.
//
func (sq *SQS) HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error) {
	return true, nil
}

// Responder is used to open a connection to an existing response queue if
// one was made available and also to provision a channel into which the
// runner can place report messages
func (sq *SQS) Responder(ctx context.Context, subscription string) (sender chan *runnerReports.Report, err kv.Error) {
	sender = make(chan *runnerReports.Report, 1)
	// Open the queue and if this cannot be done exit with the error
	go func() {
		for {
			select {
			case <-sender:
				continue
			case <-ctx.Done():
				return
			}
		}
	}()
	return sender, err
}
