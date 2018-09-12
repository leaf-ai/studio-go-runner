package main

// The file contains code for handling google certificates and
// refreshing a directory containing these certificates and using
// these to process work sent to pubsub queues that get forwarded
// to subscriptions made by the runner

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/SentientTechnologies/studio-go-runner/internal/types"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

var (
	sqsCertsDirOpt = flag.String("sqs-certs", "", "a directory used to store certificate containing sub directories")
)

type awsCred struct {
}

func (*awsCred) validate(ctx context.Context, filenames []string) (cred *runner.AWSCred, err errors.Error) {

	cred, err = runner.AWSExtractCreds(filenames)
	if err != nil {
		return cred, err
	}

	sess, errGo := session.NewSessionWithOptions(session.Options{
		Config: aws.Config{
			Region:                        aws.String(cred.Region),
			Credentials:                   cred.Creds,
			CredentialsChainVerboseErrors: aws.Bool(true),
		},
		Profile: "default",
	})

	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create a SQS client
	svc := sqs.New(sess)

	_, errGo = svc.ListQueuesWithContext(ctx, &sqs.ListQueuesInput{})
	if errGo != nil {
		return nil, errors.Wrap(errGo, "unable to list SQS queues").With("stack", stack.Trace().TrimRuntime()).With("filenames", filenames).With("region", cred.Region)
	}

	return cred, nil
}

func (awsC *awsCred) refreshAWSCert(dir string, timeout time.Duration) (project string, awsFiles []string, err errors.Error) {

	awsFiles = []string{}

	files, errGo := ioutil.ReadDir(dir)
	if errGo != nil {
		return "", awsFiles, errors.Wrap(errGo, "could not load AWS subdirectory credentials").With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
	}

	for _, credFile := range files {
		if credFile.IsDir() {
			continue
		}
		if '.' == credFile.Name()[0] {
			continue
		}
		awsFiles = append(awsFiles, filepath.Join(dir, credFile.Name()))
	}
	if len(awsFiles) != 2 {
		msg := fmt.Sprintf("subdirectory for AWS credentials contained %d not 2 files ", len(awsFiles))
		return "", []string{}, errors.New(msg).With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cred, err := awsC.validate(ctx, awsFiles)
	if err != nil {
		return "", []string{}, err
	}
	return cred.Project, awsFiles, nil
}

func (awsC *awsCred) refreshAWSCerts(dir string, timeout time.Duration) (found map[string]string, err errors.Error) {

	found = map[string]string{}

	files, errGo := ioutil.ReadDir(dir)
	if errGo != nil {
		return found, errors.Wrap(errGo, "could not load AWS credentials catalog").With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
	}

	for _, credDir := range files {
		if strings.HasPrefix(credDir.Name(), ".") {
			continue
		}
		if !credDir.IsDir() {
			continue
		}
		// Process certs and stop if any errors appear
		k, v, err := awsC.refreshAWSCert(filepath.Join(dir, credDir.Name()), timeout)
		if err != nil {
			return map[string]string{}, err
		}
		found[k] = strings.Join(v, ",")
	}

	return found, nil
}

func serviceSQS(ctx context.Context, connTimeout time.Duration) {

	if len(*sqsCertsDirOpt) == 0 {
		logger.Info("user disabled the SQS service")
		return
	}

	logger.Info("starting the SQS service")

	live := &Projects{projects: map[string]chan bool{}}

	// first time through make sure the credentials are checked immediately
	credCheck := time.Duration(time.Second)

	awsC := &awsCred{}

	// Watch for when the server should not be getting new work
	state := runner.K8sStateUpdate{
		State: types.K8sRunning,
	}

	lifecycleC := make(chan runner.K8sStateUpdate, 1)
	id, err := k8SStateUpdates().Add(lifecycleC)
	if err == nil {
		defer func() {
			k8SStateUpdates().Delete(id)
			close(lifecycleC)
		}()
	} else {
		logger.Warn(fmt.Sprint(err))
	}

	for {
		select {
		case <-ctx.Done():

			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				close(quiter)
			}
			return

		case state = <-lifecycleC:
		case <-time.After(credCheck):
			// If the pulling of work is currently suspending bail out of checking the queues
			if state.State != types.K8sRunning {
				continue
			}
			credCheck = time.Duration(15 * time.Second)

			found, err := awsC.refreshAWSCerts(*sqsCertsDirOpt, connTimeout)
			if err != nil {
				logger.Warn(fmt.Sprintf("unable to refresh AWS certs due to %v", err))
				continue
			}

			live.Lifecycle(found)
		}
	}
}
