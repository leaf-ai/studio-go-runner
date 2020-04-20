// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the generic implementation of a queue server, or a project in
// runner terminology.  As queue servers are instantiated they will generate a Projects
// structure that will track a queue server across its lifetime.
//
// This file also contains a simple project tracking value type that will accompany the
// contexts that are scoped to servicing a queue within a queue server

import (
	"context"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/studio-go-runner/internal/runner"
	"github.com/leaf-ai/studio-go-runner/internal/types"
	"github.com/prometheus/client_golang/prometheus"
	uberatomic "go.uber.org/atomic"
)

type projectContextKey string

var (
	projectKey = projectContextKey("project")

	k8sListener sync.Once
	openForBiz  = uberatomic.NewBool(true)

	wrapper         *runner.Wrapper = nil
	initWrapperOnce sync.Once
)

func initWrapper() {
	// Get the secrets that Kubernetes has stored for the runners to use
	// for their decryption of messages on the queues
	w, err := runner.KubernetesWrapper(*msgEncryptDirOpt)
	if err != nil {
		if runner.IsAliveK8s() != nil {
			logger.Warn("kubernetes missing", "error", err.Error())
			return
		}
		logger.Warn("unable to load message encryption secrets", "error", err.Error())
		return
	}
	logger.Debug("wrapper secrets loaded")
	wrapper = w
}

// NewProjectContext returns a new Context that carries a value for the project associated with the context
func NewProjectContext(ctx context.Context, proj string) context.Context {
	return context.WithValue(ctx, projectKey, proj)
}

// FromProjectContext returns the User value stored in ctx, if any.
func FromProjectContext(ctx context.Context) (proj string, wasPresent bool) {
	proj, wasPresent = ctx.Value(projectKey).(string)
	return proj, wasPresent
}

// Projects is used across several queuing modules for example the rabbitMQ server
//
type Projects struct {
	queueType string
	projects  map[string]context.CancelFunc
	sync.Mutex
}

func (*Projects) startStateWatcher(ctx context.Context) (err kv.Error) {
	lifecycleC := make(chan runner.K8sStateUpdate, 1)
	id, err := k8sStateUpdates().Add(lifecycleC)
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			k8sStateUpdates().Delete(id)
			close(lifecycleC)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case state := <-lifecycleC:
				openForBiz.Store(state.State == types.K8sRunning)
			}
		}
	}()

	return err
}

// Lifecycle is used to run a single pass across all of the found queues and subscriptions
// looking for work and any needed updates to the list of queues found within the various queue
// servers that are configured
//
// live has a list of queue references as determined by the queue implementation
// found has a map of queue references specific to the queue implementation, the key, and
// a value with credential information
//
func (live *Projects) Lifecycle(ctx context.Context, found map[string]string) (err kv.Error) {

	initWrapperOnce.Do(initWrapper)

	if len(found) == 0 {
		return kv.NewError("no queues").With("stack", stack.Trace().TrimRuntime())
	}

	if !openForBiz.Load() {
		return nil
	}

	k8sListener.Do(func() {
		err = live.startStateWatcher(ctx)
	})

	if err != nil {
		return err
	}

	// Check to see if the ctx has been fired and if so clear the found list to emulate a
	// queue server with no queues
	if ctx.Err() != nil && len(found) != 0 {
		return kv.Wrap(ctx.Err()).With("stack", stack.Trace().TrimRuntime())
	}

	live.Lock()
	defer live.Unlock()

	// Look for new projects that have been found
	for proj, cred := range found {

		queueChecked.With(prometheus.Labels{"host": host, "queue_type": live.queueType, "queue_name": proj}).Inc()

		if _, isPresent := live.projects[proj]; !isPresent {
			logger.Debug("project added", "project_id", proj, "stack", stack.Trace().TrimRuntime())

			// Now start processing the queues that exist within the project in the background,
			// but not before claiming the slot in our live project structure
			localCtx, cancel := context.WithCancel(NewProjectContext(context.Background(), proj))
			live.projects[proj] = cancel

			// Start the projects runner and let it go off and do its thing until it dies
			// or no longer has a matching credentials file
			go live.LifecycleRun(localCtx, proj[:], cred[:], wrapper)
		}
	}

	// If projects have disappeared from the queue server side then kill them from the
	// running set of projects
	for proj, quiter := range live.projects {
		if quiter != nil {
			if _, isPresent := found[proj]; !isPresent {
				logger.Info("project deleted", "project_id", proj, "stack", stack.Trace().TrimRuntime())
				quiter()

				// The cleanup will occur inside the service routine later on
				live.projects[proj] = nil
			}
		}
	}

	return err
}

// LifecycleRun runs until the ctx is Done().  ctx is treated as a queue and project
// specific context that is Done() when the queue is dropped from the server.
//
func (live *Projects) LifecycleRun(ctx context.Context, proj string, cred string, wrapper *runner.Wrapper) {
	logger.Debug("started project runner", "project_id", proj,
		"stack", stack.Trace().TrimRuntime())

	defer func(ctx context.Context, proj string) {
		live.Lock()
		delete(live.projects, proj)
		live.Unlock()

		ctxProj, wasFound := FromProjectContext(ctx)

		if wasFound && ctxProj == proj {
			logger.Debug("stopped project runner", "project_id", proj,
				"stack", stack.Trace().TrimRuntime())
		} else {
			if wasFound {
				logger.Warn("stopped project runner", "project_id", proj, "ctx_project_id", ctxProj,
					"stack", stack.Trace().TrimRuntime())
			} else {
				logger.Warn("stopped project runner", "project_id", proj, "ctx_project_id", "unknown",
					"stack", stack.Trace().TrimRuntime())
			}
		}
	}(ctx, proj)

	qr, err := NewQueuer(proj, cred, wrapper)
	if err != nil {
		logger.Warn("failed project initialization", "project", proj, "error", err.Error())
		return
	}
	if err := qr.run(ctx, 5*time.Minute, 5*time.Second); err != nil {
		logger.Warn("failed project runner", "project", proj, "error", err)
		return
	}
}
