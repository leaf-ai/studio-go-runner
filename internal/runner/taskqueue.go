// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file defines an interface for task queues used by the runner
import (
	"context"
	"os"
	"regexp"
	"strings"

	runnerReports "github.com/leaf-ai/studio-go-runner/internal/gen/dev.cognizant_dev.ai/genproto/studio-go-runner/reports/v1"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// QueueTask encapsulates the metadata needed to handle requests on a queue.
//
type QueueTask struct {
	FQProject    string // A proprietary runner label for a project to uniquely identify it
	Project      string
	QueueType    string
	Subscription string
	ShortQName   string // The short queue name for the current task, will be used to retrieve signing keys
	Credentials  string
	Msg          []byte
	Handler      MsgHandler
	Wrapper      *Wrapper                   // A store of encryption related information for messages
	ResponseQ    chan *runnerReports.Report // A response queue the runner can employ to send progress updates on
}

// MsgHandler defines the function signature for a generic message handler for a specified queue implementation
//
type MsgHandler func(ctx context.Context, qt *QueueTask) (resource *Resource, ack bool, err kv.Error)

// TaskQueue is the interface definition for a queue message handling implementation.
//
type TaskQueue interface {
	// Refresh is used to scan the catalog of queues work could arrive on and pass them back to the caller
	Refresh(ctx context.Context, qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (known map[string]interface{}, err kv.Error)

	// Process a single unit of work if available on a queue, blocking operation on the queue and on the processing
	// of the work itself
	Work(ctx context.Context, qt *QueueTask) (msgProcessed bool, resource *Resource, err kv.Error)

	// Check that the specified queue exists
	Exists(ctx context.Context, subscription string) (exists bool, err kv.Error)

	// HasWork is a probe to see if there is a potential for work to be available
	HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error)

	// Responder is used to open a connection to an existing repsonse queue if
	// one was made available and also to provision a channel into which the
	// runner can place report messages
	Responder(ctx context.Context, subscription string) (sender chan *runnerReports.Report, err kv.Error)
}

// NewTaskQueue is used to initiate processing for any of the types of queues
// the runner supports.  It also performs some lazy initialization.
//
func NewTaskQueue(project string, creds string, wrapper *Wrapper) (tq TaskQueue, err kv.Error) {

	switch {
	case strings.HasPrefix(project, "amqp://"):
		tq, err = NewRabbitMQ(project, creds, wrapper)
	default:
		// SQS uses a number of credential and config file names
		files := strings.Split(creds, ",")
		for _, file := range files {
			_, errGo := os.Stat(file)
			if errGo != nil {
				return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file).With("project", project)
			}
		}
		tq, err = NewSQS(project, creds, wrapper)
	}

	return tq, err
}
