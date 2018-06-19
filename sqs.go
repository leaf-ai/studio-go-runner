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

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
)

type SQS struct {
	project string
	creds   *AWSCred
}

func NewSQS(project string, creds string) (sqs *SQS, err errors.Error) {
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

func (sq *SQS) listQueues(qNameMatch *regexp.Regexp) (queues *sqs.ListQueuesOutput, err errors.Error) {

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:                        aws.String(sq.creds.Region),
			Credentials:                   sq.creds.Creds,
			CredentialsChainVerboseErrors: aws.Bool(true),
		},
		Profile: "default",
	})

	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)

	ctx, cancel := context.WithTimeout(context.Background(), *sqsTimeoutOpt)
	defer cancel()

	listParam := &sqs.ListQueuesInput{}

	qs, errGo := svc.ListQueuesWithContext(ctx, listParam)
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}
	if qNameMatch == nil {
		return qs, nil
	}

	queues.QueueUrls = []*string{}
	for _, qURL := range qs.QueueUrls {
		if qURL == nil {
			continue
		}
		fullURL, errGo := url.Parse(*qURL)
		if errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
		}
		paths := strings.Split(fullURL.Path, "/")
		if qNameMatch.MatchString(paths[len(paths)-1]) {
			queues.QueueUrls = append(queues.QueueUrls, qURL)
		}
	}
	return queues, nil
}

func (sq *SQS) refresh(qNameMatch *regexp.Regexp) (known []string, err errors.Error) {

	known = []string{}

	result, err := sq.listQueues(qNameMatch)
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

func (sq *SQS) Refresh(qNameMatch *regexp.Regexp, timeout time.Duration) (known map[string]interface{}, err errors.Error) {

	found, err := sq.refresh(qNameMatch)
	if err != nil {
		return known, err
	}

	known = make(map[string]interface{}, len(found))
	for _, url := range found {
		known[fmt.Sprintf("%s:%s", sq.creds.Region, url)] = sq.creds
	}

	return known, nil
}

func (sq *SQS) Exists(ctx context.Context, subscription string) (exists bool, err errors.Error) {

	queues, err := sq.listQueues(nil)
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

func (sq *SQS) Work(ctx context.Context, qTimeout time.Duration, subscription string, handler MsgHandler) (msgCnt uint64, resource *Resource, err errors.Error) {

	regionUrl := strings.SplitN(subscription, ":", 2)
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
		return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)

	qCtx, qCancel := context.WithTimeout(context.Background(), qTimeout)
	defer func() {
		defer func() {
			recover()
		}()
		qCancel()
	}()

	// Use the main context to cancel this micro context
	go func() {
		select {
		case <-ctx.Done():
			qCancel()
		}
	}()

	visTimeout := int64(30)
	waitTimeout := int64(5)
	msgs, errGo := svc.ReceiveMessageWithContext(qCtx,
		&sqs.ReceiveMessageInput{
			QueueUrl:          &url,
			VisibilityTimeout: &visTimeout,
			WaitTimeSeconds:   &waitTimeout,
		})
	if errGo != nil {
		return 0, nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}
	if len(msgs.Messages) == 0 {
		return 0, nil, nil
	}

	// Make sure that the main ctx has not been Done with before continuing
	select {
	case <-ctx.Done():
		return 0, nil, errors.New("queue worker cancel received").With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
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

	rsc, ack := handler(ctx, sq.project, url, "", []byte(*msgs.Messages[0].Body))
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

	return 1, resource, nil
}
