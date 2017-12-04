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
	sqsTimeoutOpt = flag.Duration("sqs-timeout", time.Duration(5*time.Second), "the period of time for discrete SQS operations to use for timeouts")
	sqsRegionsOpt = flag.String("sqs-regions", "us-west-2", "a comma seperated list of regions this runner will look for work on")
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
	result, errGo := svc.ListQueuesWithContext(ctx, &sqs.ListQueuesInput{})
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

func (sqs *SQS) Refresh(timeout time.Duration) (known map[string]interface{}, err errors.Error) {

	known = map[string]interface{}{}

	regions := strings.Split(*sqsRegionsOpt, ",")
	if 0 == len(regions) {
		return nil, errors.New("the --sqs-regions flag has not been set").With("stack", stack.Trace().TrimRuntime())
	}

	credFiles := strings.Split(sqs.creds, ",")
	if len(credFiles) != 2 {
		return nil, errors.New("the sqs config should contain 2 files for AWS credentials handling, but did not").With("stack", stack.Trace().TrimRuntime()).With("project", sqs.project)
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

func (sqs *SQS) Work(ctx context.Context, subscription string, handler MsgHandler) (resource *Resource, err errors.Error) {

	return nil, errors.New("no yet implemented").With("stack", stack.Trace().TrimRuntime()).With("subscription", subscription)
}
