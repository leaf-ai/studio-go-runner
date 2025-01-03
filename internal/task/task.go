// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package task

// This file defines an interface for task queues used by the runner
import (
	"context"
	"crypto/rsa"
	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/go-service/pkg/server"
	"regexp"
	"time"

	"github.com/jjeffery/kv" // MIT License
)

// QueueDesc is a simple descriptor structure for queues
type QueueDesc struct {
	Proj string
	Mgt  string
	Cred string
}

// QueueTask encapsulates the metadata needed to handle requests on a queue.
type QueueTask struct {
	FQProject    string // A proprietary runner label for a project to uniquely identify it
	Project      string
	QueueType    string
	Subscription string
	ShortQName   string // The short queue name for the current task, will be used to retrieve signing keys
	Credentials  string
	Msg          []byte
	Handler      MsgHandler
	ResponseQ    chan string // A response message queue the runner can use to send progress updates
	QueueLogger  *log.Logger
}

// MsgHandler defines the function signature for a generic message handler for a specified queue implementation
type MsgHandler func(ctx context.Context, qt *QueueTask) (resource *server.Resource, ack bool, err kv.Error)

// TaskQueue is the interface definition for a queue message handling implementation.
type TaskQueue interface {
	// Refresh is used to scan the catalog of queues work could arrive on and pass them back to the caller
	Refresh(ctx context.Context, qNameMatch *regexp.Regexp, qNameMismatch *regexp.Regexp) (known map[string]interface{}, err kv.Error)

	// Process a single unit of work if available on a queue, blocking operation on the queue and on the processing
	// of the work itself
	Work(ctx context.Context, qt *QueueTask) (msgProcessed bool, resource *server.Resource, err kv.Error)

	// Check that the specified queue exists
	Exists(ctx context.Context, subscription string) (exists bool, err kv.Error)

	// HasWork is a probe to see if there is a potential for work to be available
	HasWork(ctx context.Context, subscription string) (hasWork bool, err kv.Error)

	// Responder is used to open a connection to an existing response queue if
	// one was made available and also to provision a channel into which the
	// runner can place report messages
	Responder(ctx context.Context, subscription string, encryptKey *rsa.PublicKey) (sender chan string, err kv.Error)

	// ExtractShortQName is useful for getting the short unique queue name useful for indexing collections etc
	GetShortQName(qt *QueueTask) (shortName string, err kv.Error)

	GetQueuesRefreshInterval() time.Duration

	GetWorkCheckInterval() time.Duration
}
