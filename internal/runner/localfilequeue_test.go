// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for LocalQueue implementation.

import (
	"encoding/json"
	"github.com/go-stack/stack"
	"os"
	"testing"
	"time"

	"github.com/jjeffery/kv" // MIT License

	"github.com/leaf-ai/go-service/pkg/log"
)

type testRequest struct {
	name string
	value int
}

func Publish(server *LocalQueue, queue string, r *testRequest) (err kv.Error) {
	buf, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return server.Publish(queue, "", buf)
}

func TestFileQueue(t *testing.T) {
	dir, errGo := os.MkdirTemp("", "lfq-test")
	if errGo != nil {
		t.Fatalf("FAILED to create temp. directory: %v", errGo)
		return
	}
	defer os.RemoveAll(dir) // clean up

	logger := log.NewLogger("local-queue")
	server := NewLocalQueue(dir, nil, logger)

	queue1 := "queue1"
	req1 := testRequest{
		name: "Iam#1",
		value: 1,
	}

	req2 := testRequest{
		name: "Iam#2",
		value: 2,
	}

	req3 := testRequest{
		name: "Iam#3",
		value: 3,
	}

	err := Publish(server, queue1, &req1)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue1, &req2)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue1, &req3)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)



}
