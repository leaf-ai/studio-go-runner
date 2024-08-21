// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package aws_ext

// This file contains the implementation of AWS SQS message queues
// as they are used by studioML

import (
	"context"
	"crypto/rsa"
	"flag"
	"fmt"
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/go-service/pkg/server"

	"github.com/leaf-ai/studio-go-runner/internal/task"
	"github.com/leaf-ai/studio-go-runner/pkg/wrapper"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
	sqsAwsRegion  = flag.String("aws-region", "us-west-2", "AWS region for SQS workload queues")
)

// SQS encapsulates an AWS based SQS queue and associated it with a project
type SQS struct {
	project string                 // Fully qualified SQS queue reference
	creds   *request.AWSCredential // AWS credentials for access queues
	wrapper wrapper.Wrapper        // Decryption information for messages with encrypted payloads
	logger  *log.Logger
}

// NewSQS creates an SQS data structure using set of credentials (creds) for
// an sqs queue (sqs)
func NewSQS(project string, creds string, w wrapper.Wrapper, l *log.Logger) (queue *SQS, err kv.Error) {
	// Use the creds directory to locate all the credentials for AWS within
	// a hierarchy of directories
	if len(creds) > 0 {
		return nil, kv.NewError("Expected empty SQS credentials").With("creds", creds, "project", project)
	}

	// Create non-existent AWS credentials to force using AWS defaults.
	awsCreds := &request.AWSCredential{
		AccessKey: "",
		SecretKey: "",
		Region:    *sqsAwsRegion,
	}
	return &SQS{
		project: project,
		creds:   awsCreds,
		wrapper: w,
		logger:  l,
	}, nil
}

// GetSQSProjects can be used to get a list of the SQS servers and the main URLs that are accessible to them
func GetSQSProjects(credFiles []string, logger *log.Logger) (urls map[string]struct{}, err kv.Error) {

	var credsStr = ""
	if credFiles != nil {
		credsStr = strings.Join(credFiles, ",")
	}
	q, err := NewSQS("aws_probe", credsStr, nil, logger)
	if err != nil {
		return urls, err
	}
	found, err := q.refresh(nil, nil)
	if err != nil {
		return urls, kv.Wrap(err, "failed to refresh sqs").With("stack", stack.Trace().TrimRuntime())
	}
	logger.Debug("GetSQSProjects: found queues: %v", found)

	urls = make(map[string]struct{}, len(found))
	for _, urlStr := range found {
		qURL, err := url.Parse(urlStr)
		if err != nil {
			logger.Debug("Failed to parse queue url: %s err: %s", urlStr, err.Error())
			continue
		}
		segments := strings.Split(qURL.Path, "/")
		qURL.Path = strings.Join(segments[:len(segments)-1], "/")
		urls[qURL.String()] = struct{}{}
	}

	return urls, nil
}

func (sq *SQS) listQueues(qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (queues *sqs.ListQueuesOutput, err kv.Error) {

	cfg, err := sq.creds.CreateAWSConfig("")
	if err != nil {
		return nil, err.With("stack", stack.Trace().TrimRuntime())
	}
	sqsClient := sqs.NewFromConfig(*cfg)
	output, errGo := sqsClient.ListQueues(context.Background(), &sqs.ListQueuesInput{})
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	queues = &sqs.ListQueuesOutput{
		QueueUrls: []string{},
	}
	for _, qURL := range output.QueueUrls {
		if len(qURL) == 0 {
			continue
		}
		fullURL, errGo := url.Parse(qURL)
		if errGo != nil {
			return nil, kv.Wrap(errGo).With("qurl", qURL).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
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

	for _, url := range result.QueueUrls {
		// Avoid using an empty URL string
		if len(url) == 0 {
			continue
		}
		known = append(known, url)
	}
	return known, nil
}

// Refresh uses a regular expression to obtain matching queues from
// the configured SQS server on AWS (sqs).
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
func (sq *SQS) Exists(ctx context.Context, subscription string) (exists bool, err kv.Error) {

	queues, err := sq.listQueues(nil, nil)
	if err != nil {
		return true, err
	}

	if sq.logger != nil {
		sq.logger.Debug("SQS-QUEUE LIST: ", "subscription", subscription)
		for _, q := range queues.QueueUrls {
			sq.logger.Debug("    listed queue URL:", q)
		}
	}
	// Our SQS subscription (queue name) has a form:
	// "region":"queue-name"
	// We are using queue name only for matching.
	segments := strings.Split(subscription, ":")
	queueName := segments[len(segments)-1]
	for _, q := range queues.QueueUrls {
		if strings.HasSuffix(q, queueName) {
			return true, nil
		}
	}
	return false, nil
}

func visError(err error) string {
	if err == nil {
		return "none"
	}
	return err.Error()
}

// Work is invoked by the queue handling software within the runner to get the
// specific queue implementation to process potential work that could be
// waiting inside the queue.
func (sq *SQS) Work(ctx context.Context, qt *task.QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error) {

	regionUrl := strings.SplitN(qt.Subscription, ":", 2)
	urlString := sq.project + "/" + regionUrl[1]
	items := strings.Split(urlString, "/")
	qt.ShortQName = items[len(items)-1]
	hostName, _ := os.Hostname()

	cfg, err := sq.creds.CreateAWSConfig(urlString)
	if err != nil {
		return false, nil, err.With("stack", stack.Trace().TrimRuntime())
	}
	// Create a SQS service client.
	sqsClient := sqs.NewFromConfig(*cfg)

	defer func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("panic in producer %#v, %s\n", r, string(debug.Stack()))
			}
		}()
	}()

	visTimeout := int32(30)
	waitTimeout := int32(5)

	params := &sqs.ReceiveMessageInput{
		QueueUrl:            &urlString,
		MaxNumberOfMessages: 1,
		WaitTimeSeconds:     waitTimeout,
		VisibilityTimeout:   visTimeout,
	}

	// Receive messages
	msgs, errGo := sqsClient.ReceiveMessage(context.Background(), params)
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

	var taskMessage *sqs.Message = nil

	msgForceDeleted := false
	hardVisibilityTimeout := 12*time.Hour - 10*time.Minute
	visExtensionLimit := time.Now().Add(hardVisibilityTimeout)
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
				if visExtensionLimit.Before(time.Now()) {
					// Message has reached hard SQS visibility timeout,
					// and processing still not finished.
					// Our approach here is to delete the message to avoid task resubmit
					// and continue processing.
					_, errGo := sqsClient.DeleteMessage(
						context.Background(),
						&sqs.DeleteMessageInput{
							QueueUrl:      &urlString,
							ReceiptHandle: taskMessage.ReceiptHandle,
						})
					msgForceDeleted = true
					if qt.QueueLogger != nil {
						qt.QueueLogger.Debug("SQS-QUEUE: Hard SQS visibility limit reached. DELETE msg from queue: ", qt.ShortQName, "error", visError(errGo), "host: ", hostName)
					}
					return
				}

				if _, errGo := svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
					QueueUrl:          &urlString,
					ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
					VisibilityTimeout: &visTimeout,
				}); errGo != nil {
					// Once the 1/2 way mark is reached continue to try to change the
					// visibility at decreasing intervals until we finish the job
					if timeout.Seconds() > 5.0 {
						timeout = time.Duration(timeout / 2)
					}
				}
				if qt.QueueLogger != nil {
					qt.QueueLogger.Debug("SQS-QUEUE: Visibility timeout extended for queue: ", qt.ShortQName, "error", visError(errGo), "host: ", hostName)
				}

			case <-quitC:
				return
			}
		}
	}()

	taskMessage = msgs.Messages[0]
	qt.Msg = nil
	qt.Msg = []byte(*taskMessage.Body)

	rsc, ack, err := qt.Handler(ctx, qt)
	errMsg := "no error"
	if err != nil {
		errMsg = err.Error()
	}
	close(quitC)

	if !msgForceDeleted {
		if ack {
			// Delete the message
			svc.DeleteMessage(&sqs.DeleteMessageInput{
				QueueUrl:      &urlString,
				ReceiptHandle: taskMessage.ReceiptHandle,
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
	} else {
		if qt.QueueLogger != nil {
			qt.QueueLogger.Debug("SQS-QUEUE: msg already FORCE DELETED from queue: ", qt.ShortQName, "err: ", errMsg, "host: ", hostName)
		}
	}

	return true, resource, err
}

// HasWork will look at the SQS queue to see if there is any pending work.  The function
// is called in an attempt to see if there is any point in processing new work without a
// lot of overhead.  In the case of SQS at the moment we always assume there is work.
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
func (sq *SQS) Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan string, err kv.Error) {
	sender = make(chan string, 1)
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

func (sq *SQS) GetQueuesRefreshInterval() time.Duration {
	return 5 * time.Minute
}

func (sq *SQS) GetWorkCheckInterval() time.Duration {
	return 5 * time.Second
}
