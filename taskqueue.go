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
	// Refresh is used to
	Refresh(timeout time.Duration) (known map[string]interface{}, err errors.Error)

	Work(ctx context.Context, subscription string, handler MsgHandler) (resource *Resource, err errors.Error)
}

func NewTaskQueue(project string, creds string) (tq TaskQueue, err errors.Error) {
	// The Google creds will come down as .json files, AWS will be a number of credential and config file names
	switch {
	case strings.HasSuffix(creds, ".json"):
		return NewPubSub(project, creds)
	default:
		files := strings.Split(creds, ",")
		for _, file := range files {
			_, errGo := os.Stat(file)
			if errGo != nil {
				return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", file)
			}
		}
		return NewSQS(project, creds)
	}
	//return nil, errors.New("supplied credentials were not in a recognized format").With("stack", stack.Trace().TrimRuntime())
}
