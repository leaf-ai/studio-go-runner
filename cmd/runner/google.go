// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// The file contains code for handling google certificates and
// refreshing a directory containing these certificates and using
// these to process work sent to pubsub queues that get forwarded
// to subscriptions made by the runner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"golang.org/x/net/context"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

var (
	jsonMatch = regexp.MustCompile(`\.json$`)
)

type googleCred struct {
	CredType string `json:"type"`
	Project  string `json:"project_id"`
}

func (*googleCred) validateCred(ctx context.Context, filename string, scopes []string) (project string, err kv.Error) {

	b, errGo := ioutil.ReadFile(filepath.Clean(filename))
	if errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	cred := &googleCred{}
	if errGo = json.Unmarshal(b, cred); errGo != nil {
		return "", kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	if len(cred.Project) == 0 {
		return "", kv.NewError("bad file format for credentials").With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}

	client, errGo := pubsub.NewClient(ctx, cred.Project, option.WithCredentialsFile(filename))
	if errGo != nil {
		return "", kv.Wrap(errGo, "could not verify credentials").With("stack", stack.Trace().TrimRuntime()).With("file", filename)
	}
	client.Close()

	return cred.Project, nil
}

func refreshGoogleCerts(dir string, timeout time.Duration) (found map[string]string) {

	found = map[string]string{}

	// it is possible that google certificates are not being used currently so simply return
	// if we are not going to find any
	if _, err := os.Stat(dir); err != nil {
		logger.Trace(fmt.Sprintf("%v", err.Error()))
		return found
	}

	gCred := &googleCred{}

	filepath.Walk(dir, func(path string, f os.FileInfo, _ error) error {
		if !f.IsDir() {
			if jsonMatch.MatchString(f.Name()) {
				// Check if this is a genuine credential
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				project, err := gCred.validateCred(ctx, path, []string{})
				if err != nil {
					logger.Warn(err.Error())
					return nil
				}

				// If so include it
				found[project] = path
			} else {
				logger.Trace(fmt.Sprintf("did not match %s (%s)", f.Name(), path))
			}
		}
		return nil
	})

	if len(found) == 0 {
		logger.Info(fmt.Sprintf("no google certs found at %s", dir))
	}

	return found
}

func servicePubsub(ctx context.Context, connTimeout time.Duration) {

	live := &Projects{
		queueType: "pubsub",
		projects:  map[string]context.CancelFunc{},
	}

	// first time through make sure the credentials are checked immediately
	credCheck := time.Duration(time.Second)

	// Watch for when the server should not be getting new work
	state := runner.K8sStateUpdate{
		State: types.K8sRunning,
	}

	lifecycleC := make(chan runner.K8sStateUpdate, 1)
	id, err := k8sStateUpdates().Add(lifecycleC)
	if err == nil {
		defer func() {
			k8sStateUpdates().Delete(id)
			close(lifecycleC)
		}()
	} else {
		logger.Warn(fmt.Sprint(err))
	}

	host, errGo := os.Hostname()
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
		case <-time.After(credCheck):
			// If the pulling of work is currently suspending bail out of checking the queues
			if state.State != types.K8sRunning {
				queueIgnored.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": ""}).Inc()
				continue
			}
			credCheck = time.Duration(15 * time.Second)

			dir, errGo := filepath.Abs(*googleCertsDirOpt)
			if errGo != nil {
				logger.Warn(fmt.Sprintf("%#v", errGo))
				continue
			}

			found := refreshGoogleCerts(dir, connTimeout)

			if len(found) != 0 {
				logger.Trace(fmt.Sprintf("checking google certs in %s returned %v", dir, found))
				credCheck = time.Duration(time.Minute)
				continue
			}

			live.Lifecycle(ctx, found)
		}
	}
}
