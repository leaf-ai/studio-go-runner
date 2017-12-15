package runner

// This file contains the implementation of AWS SQS message queues
// as they are used by studioML

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(15*time.Second), "the period of time for discrete SQS operations to use for timeouts")
	sqsRegionsOpt = flag.String("sqs-regions", "us-west-2", "a comma seperated list of regions this runner will look for work on")
	sqsPrefixOpt  = flag.String("sqs-prefix", "sqs_StudioML", "a fixed prefix for queue names that the runner will look for work on")
)

type SQS struct {
	project string
	creds   string
}

func NewSQS(project string, creds string) (sqs *SQS, err errors.Error) {
	// Use the creds directory to locate all of the credentials for AWS within
	// a hierarchy of directories

	return &SQS{
		project: project,
		creds:   creds,
	}, nil
}

func refreshRegion(region string, credFiles []string) (known []string, err errors.Error) {
	known = []string{}

	// In the case of SQS the credentials are going to arrive in two files,
	// the AWS shared CFG file, amd the AWS shared credentials file.  This code
	// uses a comma seperated list for the two and the shared cfg file is first.  I dont
	// known if this is significant but that is how the AWS go SDK orders them and so we
	// will follow suit here.
	sess, errGo := session.NewSessionWithOptions(session.Options{
		Profile:           "default",
		SharedConfigState: session.SharedConfigEnable,
		SharedConfigFiles: credFiles,
	})

	if errGo != nil {
		return known, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", credFiles).With("region", region)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)

	ctx, cancel := context.WithTimeout(context.Background(), *sqsTimeoutOpt)
	defer cancel()
	result, errGo := svc.ListQueuesWithContext(ctx,
		&sqs.ListQueuesInput{
			QueueNamePrefix: sqsPrefixOpt,
		})
	if errGo != nil {
		return known, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", credFiles).With("region", region)
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

func (sq *SQS) Refresh(timeout time.Duration) (known map[string]interface{}, err errors.Error) {

	known = map[string]interface{}{}

	regions := strings.Split(*sqsRegionsOpt, ",")
	if 0 == len(regions) {
		return nil, errors.New("the --sqs-regions flag has not been set").With("stack", stack.Trace().TrimRuntime())
	}

	credFiles := strings.Split(sq.creds, ",")
	if len(credFiles) != 2 {
		return nil, errors.New("the sqs config should contain 2 files for AWS credentials handling, but did not").With("stack", stack.Trace().TrimRuntime()).With("project", sq.project)
	}

	for _, region := range regions {
		regionKnown, err := refreshRegion(region, credFiles)
		if err != nil {
			return known, err
		}
		for _, aKnown := range regionKnown {
			known[fmt.Sprintf("%s:%s", region, aKnown)] = credFiles
		}
	}

	return known, nil
}

func (sq *SQS) Exists(ctx context.Context, subscription string) (exists bool, err errors.Error) {

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Profile:           "default",
		SharedConfigState: session.SharedConfigEnable,
		SharedConfigFiles: strings.Split(sq.creds, ","),
	})

	if errGo != nil {
		return true, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}

	// Create a SQS service client.
	svc := sqs.New(sess)
	queues, errGo := svc.ListQueuesWithContext(ctx,
		&sqs.ListQueuesInput{
			QueueNamePrefix: &subscription,
		})
	if errGo != nil {
		return true, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("credentials", sq.creds)
	}
	for _, q := range queues.QueueUrls {
		if q != nil {
			if strings.HasSuffix(*q, subscription) {
				NewLogger("sqs").Debug(fmt.Sprintf("%s exists", subscription))
				return true, nil
			}
		}
	}
	return false, nil
}

func (sq *SQS) Work(ctx context.Context, qTimeout time.Duration, subscription string, handler MsgHandler) (msgCnt uint64, resource *Resource, err errors.Error) {

	regionUrl := strings.SplitN(subscription, ":", 2)

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Profile:           "default",
		SharedConfigState: session.SharedConfigEnable,
		SharedConfigFiles: strings.Split(sq.creds, ","),
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
			QueueUrl:          &regionUrl[1],
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
					QueueUrl:          &regionUrl[1],
					ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
					VisibilityTimeout: &visTimeout,
				})
			case <-quitC:
				return
			}
		}
	}()

	rsc, ack := handler(ctx, sq.project, regionUrl[1], sq.creds, []byte(*msgs.Messages[0].Body))
	close(quitC)

	if ack {
		// Delete the message
		svc.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      &regionUrl[1],
			ReceiptHandle: msgs.Messages[0].ReceiptHandle,
		})
		resource = rsc
	} else {
		// Set visibility timeout to 0, in otherwords Nack the message
		visTimeout = 0
		svc.ChangeMessageVisibility(&sqs.ChangeMessageVisibilityInput{
			QueueUrl:          &regionUrl[1],
			ReceiptHandle:     msgs.Messages[0].ReceiptHandle,
			VisibilityTimeout: &visTimeout,
		})
	}

	return 1, resource, nil
}
