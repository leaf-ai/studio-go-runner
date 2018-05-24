package runner

// This file defines an interface for task queues used by the runner
import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// convert types take an int and return a string value.
type MsgHandler func(ctx context.Context, project string, subscription string, credentials string, data []byte) (resource *Resource, ack bool)

type TaskQueue interface {
	// Refresh is used to scan the catalog of queues work could arrive on and pass them back to the caller
	Refresh(timeout time.Duration) (known map[string]interface{}, err errors.Error)

	// Process a unit of work after it arrives on a queue
	Work(ctx context.Context, qTimeout time.Duration, subscription string, handler MsgHandler) (msgs uint64, resource *Resource, err errors.Error)

	// Check that the specified queue exists
	Exists(ctx context.Context, subscription string) (exists bool, err errors.Error)
}

func NewTaskQueue(project string, creds string) (tq TaskQueue, err errors.Error) {
	// The Google creds will come down as .json files, AWS will be a number of credential and config file names
	switch {
	case strings.HasSuffix(creds, ".json"):
		return NewPubSub(project, creds)
	case strings.HasPrefix(project, "amqp://"):
		return NewRabbitMQ(project, creds)
	default:
		files := strings.Split(creds, ",")
		for _, file := range files {
			_, errGo := os.Stat(file)
			if errGo != nil {
				return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file).With("project", project)
			}
		}
		return NewSQS(project, creds)
	}
}
