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
	"errors"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/studio-go-runner/internal/defense"
	"github.com/leaf-ai/studio-go-runner/internal/task"
	uberatomic "go.uber.org/atomic"
	"sync"
)

type projectContextKey string

var (
	projectKey = projectContextKey("project")

	openForBiz = uberatomic.NewBool(true)

	encryptWrap     *defense.Wrapper = nil
	encryptWrapErr                   = kv.Wrap(errors.New("wrapper uninitialized"))
	initWrapperOnce sync.Once
)

func initWrapper() {

	defer func() {
		if r := recover(); r != nil {
			logger.Warn("recovered", "cause", r)
		}
	}()
	// Get the secrets that Kubernetes has stored for the runners to use
	// for their decryption of messages on the queues
	w, err := defense.KubernetesWrapper(*msgEncryptDirOpt)
	if err != nil {
		if server.IsAliveK8s() != nil {
			logger.Warn("kubernetes missing", "error", err.Error())
			encryptWrapErr = err
			return
		}
		logger.Warn("unable to load message encryption secrets", "error", err.Error())
		encryptWrapErr = err
		return
	}
	logger.Info("wrapper secrets loaded")

	encryptWrapErr = nil
	encryptWrap = w
}

func getWrapper() (w *defense.Wrapper, err kv.Error) {

	initWrapperOnce.Do(initWrapper)

	// Make sure that clear text is permitted before continuing
	// after an error
	if encryptWrapErr != nil {
		logger.Warn("getWrapper", "stack", stack.Trace().TrimRuntime())
		if !*acceptClearTextOpt {
			return nil, encryptWrapErr
		}
		// If the runner was started with an explicitly set empty directory
		// for the credentials then it is rational to continue without
		// credentials
		if len(*msgEncryptDirOpt) == 0 {
			return nil, nil
		}
		return nil, encryptWrapErr
	}
	return encryptWrap, nil
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
type Projects struct {
	queueType string
	projects  map[string]context.CancelFunc
	sync.Mutex
}

// Cycle is used to run a single pass across all of the found queues and subscriptions
// looking for work and any needed updates to the list of queues found within the various queue
// servers that are configured.
//
// # Cycle is initiated by the queue implementation
//
// live has a list of queue references as determined by the queue implementation
// found has a map of queue references specific to the queue implementation, the key, and
// a value with credential information
func (live *Projects) Cycle(ctx context.Context, found map[string]task.QueueDesc) (err kv.Error) {

	if len(found) == 0 {
		return kv.NewError("no queues").With("stack", stack.Trace().TrimRuntime())
	}

	if !openForBiz.Load() {
		return nil
	}

	// Check to see if the ctx has been fired and if so clear the found list to emulate a
	// queue server with no queues
	if ctx.Err() != nil && len(found) != 0 {
		return kv.Wrap(ctx.Err()).With("stack", stack.Trace().TrimRuntime())
	}

	w, err := getWrapper()
	if err != nil && !*acceptClearTextOpt {
		return err
	}

	live.Lock()
	defer live.Unlock()

	// Look for new projects that have been found
	for proj, desc := range found {
		if _, isPresent := live.projects[proj]; !isPresent {
			logger.Debug("project added", "project_id", proj, "stack", stack.Trace().TrimRuntime())

			// Now start processing the queues that exist within the project in the background,
			// but not before claiming the slot in our live project structure
			localCtx, cancel := context.WithCancel(NewProjectContext(context.Background(), proj))
			live.projects[proj] = cancel

			// Start the projects runner and let it go off and do its thing until it dies
			// or no longer has a matching credentials file
			go live.run(localCtx, proj[:], desc.Mgt[:], desc.Cred[:], w)
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

	return nil
}

// run treats ctx as a queue and project specific context that is Done() when the
// queue is dropped from the server.
func (live *Projects) run(ctx context.Context, proj string, mgt string, cred string, w *defense.Wrapper) {
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
			projName := "unknown"
			if wasFound {
				projName = ctxProj
			}
			logger.Warn("stopped project runner", "project_id", proj, "ctx_project_id", projName, "stack", stack.Trace().TrimRuntime())
		}
	}(ctx, proj)

	qr, err := NewQueuer(proj, mgt, cred, w)
	if err != nil {
		logger.Warn("failed project initialization", "project", proj, "error", err.Error())
		return
	}
	if err := qr.run(ctx); err != nil {
		logger.Warn("failed project runner", "project", proj, "error", err.Error())
		return
	}
}
