// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// The file contains code for handling aws certificates and
// refreshing a directory containing these certificates and using
// these to process work sent to SQS queues that get forwarded
// to subscriptions made by the runner

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"

	//"github.com/prometheus/client_golang/prometheus"

	"github.com/andreidenissov-cog/go-service/pkg/aws_gsc"
	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/andreidenissov-cog/go-service/pkg/types"

	"github.com/leaf-ai/studio-go-runner/internal/task"
	aws_ext "github.com/leaf-ai/studio-go-runner/pkg/aws"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	sqsCertsDirOpt = flag.String("sqs-certs", "", "a directory used to store certificate containing sub directories")
)

type awsCred struct {
}

func (*awsCred) validate(ctx context.Context, filenames []string) (cred *aws_gsc.AWSCred, err kv.Error) {

	cred, err = aws_gsc.AWSExtractCreds(filenames, "default")
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
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// Create a SQS client
	svc := sqs.New(sess)

	_, errGo = svc.ListQueuesWithContext(ctx, &sqs.ListQueuesInput{})
	if errGo != nil {
		return nil, kv.Wrap(errGo, "unable to list SQS queues").With("stack", stack.Trace().TrimRuntime()).With("filenames", filenames).With("region", cred.Region)
	}

	return cred, nil
}

func (awsC *awsCred) refreshAWSCert(dir string, timeout time.Duration) (project string, awsFiles []string, err kv.Error) {

	awsFiles = []string{}

	files, errGo := ioutil.ReadDir(dir)
	if errGo != nil {
		return "", awsFiles, kv.Wrap(errGo, "could not load AWS subdirectory credentials").With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
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
		return "", []string{}, kv.NewError(msg).With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cred, err := awsC.validate(ctx, awsFiles)
	if err != nil {
		return "", []string{}, err
	}
	return cred.Project, awsFiles, nil
}

func (awsC *awsCred) refreshAWSCerts(dir string, timeout time.Duration) (found map[string]string, err kv.Error) {

	found = map[string]string{}

	files, errGo := ioutil.ReadDir(dir)
	if errGo != nil {
		return found, kv.Wrap(errGo, "could not load AWS credentials catalog").With("stack", stack.Trace().TrimRuntime()).With("directory", dir)
	}

	for _, credDir := range files {
		if strings.HasPrefix(credDir.Name(), ".") {
			continue
		}
		if !credDir.IsDir() {
			continue
		}
		// Process certs and stop if any kv.appear
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

	live := &Projects{
		queueType: "sqs",
		projects:  map[string]context.CancelFunc{},
	}

	// first time through make sure the credentials are checked immediately
	credCheck := time.Duration(time.Second)

	awsC := &awsCred{}

	// Watch for when the server should not be getting new work
	state := server.K8sStateUpdate{
		State: types.K8sRunning,
	}

	lifecycleC := make(chan server.K8sStateUpdate, 1)
	id, err := server.K8sStateUpdates().Add(lifecycleC)
	if err == nil {
		defer func() {
			server.K8sStateUpdates().Delete(id)
			close(lifecycleC)
		}()
	} else {
		logger.Warn(fmt.Sprint(err))
	}

	//host, errGo := os.Hostname()
	_, errGo := os.Hostname()
	if errGo != nil {
		logger.Warn(errGo.Error())
	}

	for {
		select {
		case <-ctx.Done():

			live.Lock()
			defer live.Unlock()

			// When shutting down stop all projects
			for _, quiter := range live.projects {
				if quiter != nil {
					quiter()
				}
			}
			return

		case state = <-lifecycleC:
			logger.Debug(fmt.Sprintf("Got Lifecycle update: state = %+v", state))

		case <-time.After(credCheck):
			// Using SQS logically doesn't depend on running under K8S host,
			// so this check is not needed:
			// If the pulling of work is currently suspending bail out of checking the queues
			//if state.State != types.K8sRunning {
			//	queueIgnored.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": ""}).Inc()
			//    logger.Debug(fmt.Sprintf("Credcheck ignored because of state = %+v", state))
			//	continue
			//}
			credCheck = time.Duration(30 * time.Second)

			found, err := awsC.refreshAWSCerts(*sqsCertsDirOpt, connTimeout)
			if err != nil {
				logger.Warn(fmt.Sprintf("unable to refresh AWS certs due to %v", err))
				continue
			}

			serverFound := make(map[string]task.QueueDesc, len(found))

			// Iterate the region for the main URLs to be used and use that as our main project key
			for _, credFiles := range found {
				urls, err := aws_ext.GetSQSProjects(strings.Split(credFiles, ","))
				if err != nil {
					logger.Warn("unable to refresh AWS certs", "error", err.Error())
					continue
				}
				for k := range urls {
					serverFound[k] = task.QueueDesc{
						Cred: credFiles,
						Proj: k,
					}
				}
			}

			logger.Info("Starting SQS lifecycle", "found", serverFound)

			if err = live.Cycle(ctx, serverFound); err != nil {
				logger.Warn("unable to process new projects", "type", live.queueType, "error", err.Error(), "stack", stack.Trace().TrimRuntime())
				continue
			}
		}
	}
}
