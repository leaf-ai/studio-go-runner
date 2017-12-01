package runner

// This file defines an interface for task queues used by the runner
import (
	"context"
	"time"

	"github.com/karlmutch/errors"
)

// convert types take an int and return a string value.
type MsgHandler func(ctx context.Context, project string, subscription string, credentials string, data []byte) (resource *Resource, ack bool)

type TaskQueue interface {
	// Refresh is used to
	Refresh(project string, credentials string, timeout time.Duration) (known map[string]interface{}, err errors.Error)

	Work(ctx context.Context, project string, subscription string, credentials string, handler MsgHandler) (resource *Resource, err errors.Error)
}

func NewTaskQueue(project string, creds string) (tq TaskQueue, err errors.Error) {
	return &PubSub{}, nil
}
