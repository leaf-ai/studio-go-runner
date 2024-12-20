// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

// This file contains the definition of task Processor interface
// and factory for constructing Processor instance appropriate for specific workload request.

import (
	"context"
	"github.com/jjeffery/kv" // MIT License
	"github.com/leaf-ai/studio-go-runner/internal/request"
	"github.com/leaf-ai/studio-go-runner/internal/task"
)

type TaskProcessor interface {
	Process(ctx context.Context) (ack bool, err kv.Error)

	GetRequest() *request.Request
	SetRequest(req *request.Request)

	GetRootDir() string

	Close() (err error)
}

// unpackMsg will use the message payload inside the queueTask (qt) and transform it into a payload
// being returned, handling any validation and decryption needed
//
func unpackMsg(qt *task.QueueTask) (r *request.Request, hardError bool, err kv.Error) {

	if !*acceptClearTextOpt {
		return nil, true, kv.NewError("unencrypted messages not enabled").With("stack", stack.Trace().TrimRuntime())
	}
	// restore the msg into the processing data structure from the JSON queue payload
	if r, err = request.UnmarshalRequest(qt.Msg); err != nil {
		return nil, true, err
	}
	return r, false, nil
}

func GetNewProcessor(ctx context.Context, qt *task.QueueTask, accessionID string) (proc TaskProcessor, hardError bool, err kv.Error) {
	// First, unpack incoming message:
	var req *request.Request = nil
	if req, hardError, err = unpackMsg(qt); hardError || err != nil {
		return nil, hardError, err
	}
	// Decide what processor do we need for that request:
	if proc, hardError, err = newProcessor(ctx, qt, accessionID); hardError || err != nil {
		return nil, hardError, err
	}
	proc.SetRequest(req)

	//return nil, true, kv.NewError("unable to determine execution class from artifacts").With("stack", stack.Trace().TrimRuntime()).
	//	With("mode", mode, "project", proc.Request.Config.Database.ProjectId).With("experiment", proc.Request.Experiment.Key)
	return proc, false, nil
}
