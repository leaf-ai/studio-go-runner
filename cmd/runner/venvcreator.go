// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the implementation of a main processing loop
// for handling pubsub messages and dispatching then after extracting data
// from firebase

//import (
//	"context"
//	"github.com/leaf-ai/studio-go-runner/internal/request"
//	pkgResources "github.com/leaf-ai/studio-go-runner/internal/resources"
//	"github.com/leaf-ai/studio-go-runner/internal/runner"
//	shortid "github.com/leaf-ai/studio-go-runner/pkg/go-shortid"
//	"io/ioutil"
//	"os"
//	"sync"
//
//	"github.com/go-stack/stack"
//	"github.com/jjeffery/kv" // MIT License
//)

// newProcessor will parse the inbound message and then validate that there are
// sufficient resources to run an experiment and then create a new working directory.
//
//func newProcessor(ctx context.Context, qt *task.QueueTask, accessionID string) (proc *processor, hardError bool, err kv.Error) {

// When a processor is initialized make sure that the logger is enabled first time through
//
//	cacheReport.Do(func() {
//		go cacheReporter(ctx)
//	})
//
//	temp, err := makeCWD()
//	if err != nil {
//		return nil, false, err
//	}
//
//	// Processors share the same root directory and use acccession numbers on the experiment key
//	// to avoid collisions
//	//
//	proc = &processor{
//		RootDir:     temp,
//		Group:       qt.Subscription,
//		QueueCreds:  qt.Credentials[:],
//		AccessionID: accessionID,
//		ResponseQ:   qt.ResponseQ,
//		evalDone:    false,
//	}
//
//	// Extract processor information from the message received on the wire, includes decryption etc
//	if hardError, err = proc.unpackMsg(qt); hardError == true || err != nil {
//		return proc, hardError, err
//	}
//
//	// Recheck the alloc using the encrypted resource description
//	if _, err = proc.allocate(false); err != nil {
//		return proc, false, err
//	}
//
//	if _, err = proc.mkUniqDir(); err != nil {
//		return proc, false, err
//	}
//
//	// Determine the type of execution that is needed for this job by
//	// inspecting the artifacts specified
//	//
//	mode := ExecUnknown
//	for group := range proc.Request.Experiment.Artifacts {
//		if len(group) == 0 {
//			continue
//		}
//		switch group {
//		case "workspace":
//			if mode == ExecUnknown {
//				mode = ExecPythonVEnv
//			}
//		}
//	}
//
//	switch mode {
//	case ExecPythonVEnv:
//		if proc.Executor, err = runner.NewVirtualEnv(proc.Request, proc.ExprDir, proc.AccessionID, logger); err != nil {
//			return nil, true, err
//		}
//	default:
//		return nil, true, kv.NewError("unable to determine execution class from artifacts").With("stack", stack.Trace().TrimRuntime()).
//			With("mode", mode, "project", proc.Request.Config.Database.ProjectId).With("experiment", proc.Request.Experiment.Key)
//	}
//	return proc, false, nil
//}
