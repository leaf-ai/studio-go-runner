// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package aws_ext

// This file contains the implementation of AWS SQS message queues
// as they are used by studioML

import (
	"context"
	"crypto/rsa"
	"flag"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/leaf-ai/go-service/pkg/aws_gsc"
	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/go-service/pkg/server"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	"github.com/leaf-ai/studio-go-runner/internal/task"
	"github.com/leaf-ai/studio-go-runner/pkg/wrapper"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
)

// SQS encapsulates an AWS based SQS queue and associated it with a project
//
type SQS struct {
	project string           // Fully qualified SQS queue reference
	creds   *aws_gsc.AWSCred // AWS credentials for access queues
	wrapper wrapper.Wrapper  // Decryption information for messages with encrypted payloads
	logger  *log.Logger
}

// NewSQS creates an SQS data structure using set set of credentials (creds) for
// an sqs queue (sqs)
//
func NewSQS(project string, creds string, w wrapper.Wrapper, l *log.Logger) (queue *SQS, err kv.Error) {
	// Use the creds directory to locate all of the credentials for AWS within
	// a hierarchy of directories

	awsCreds, err := aws_gsc.AWSExtractCreds(strings.Split(creds, ","), "default")
	if err != nil {
		return nil, err
	}

	return &SQS{
		project: project,
		creds:   awsCreds,
		wrapper: w,
		logger: l,
	}, nil
}

// GetSQSProjects can be used to get a list of the SQS servers and the main URLs that are accessible to them
func GetSQSProjects(credFiles []string) (urls map[string]struct{}, err kv.Error) {

	q, err := NewSQS("aws_probe", strings.Join(credFiles, ","), nil, nil)
	if err != nil {
		return urls, err
	}
	found, err := q.refresh(nil, nil)
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

	if sq.logger != nil {
		sq.logger.Debug("SQS-QUEUE LIST: ", "subscription", subscription)
		for _, q := range queues.QueueUrls {
			if q != nil {
				sq.logger.Debug("    listed queue URL:", *q)
			}
		}
	}
	// Our SQS subscription (queue name) has a form:
	// "region":"queue-name"
	// We are using queue name only for matching.
	segments := strings.Split(subscription, ":")
	queueName := segments[len(segments)-1]
	for _, q := range queues.QueueUrls {
		if q != nil {
			if strings.HasSuffix(queueName, *q) {
				return true, nil
			}
		}
	}
	return false, nil
}

// Work is invoked by the queue handling software within the runner to get the
// specific queue implementation to process potential work that could be
// waiting inside the queue.
func (sq *SQS) Work(ctx context.Context, qt *task.QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error) {

	regionUrl := strings.SplitN(qt.Subscription, ":", 2)
	urlString := sq.project + "/" + regionUrl[1]

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
			QueueUrl:          &urlString,
			VisibilityTimeout: &visTimeout,
			WaitTimeSeconds:   &waitTimeout,
		})
	if errGo != nil {
		return false, nil, kv.Wrap(errGo).With("credentials", sq.creds, "url", urlString).With("stack", stack.Trace().TrimRuntime())
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
					QueueUrl:          &urlString,
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

	items := strings.Split(urlString, "/")
	qt.ShortQName = items[len(items)-1]

	rsc, ack, err := qt.Handler(ctx, qt)
	errMsg := "no error"
	if err != nil {
		errMsg = err.Error()
	}
	close(quitC)

	hostName, _ := os.Hostname()
	if ack {
		// Delete the message
		svc.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      &urlString,
			ReceiptHandle: msgs.Messages[0].ReceiptHandle,
		})
		if qt.QueueLogger != nil {
			qt.QueueLogger.Debug("SQS-QUEUE: DELETE msg from queue: ", qt.ShortQName, "err: ", errMsg, "host: ", hostName)
		}
		resource = rsc
	} else {
		// Set visibility timeout to 0, in other words Nack the message
		visTimeout = 0
		svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
			QueueUrl:          &urlString,
			ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
			VisibilityTimeout: &visTimeout,
		})
		if qt.QueueLogger != nil {
			qt.QueueLogger.Debug("SQS-QUEUE: RETURN msg to queue: ", qt.ShortQName, "err: ", errMsg, "host: ", hostName)
		}
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

func (sq *SQS) GetShortQName(qt *task.QueueTask) (shortName string, err kv.Error) {
	regionUrl := strings.SplitN(qt.Subscription, ":", 2)
	urlString := sq.project + "/" + regionUrl[1]
	items := strings.Split(urlString, "/")

	return items[len(items)-1], nil
}

// Responder is used to open a connection to an existing response queue if
// one was made available and also to provision a channel into which the
// runner can place report messages
func (sq *SQS) Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan *runnerReports.Report, err kv.Error) {
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
