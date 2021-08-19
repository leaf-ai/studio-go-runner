// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for LocalQueue implementation.

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jjeffery/kv" // MIT License

	"github.com/leaf-ai/go-service/pkg/log"
)

type TestRequest struct {
	name string
	value int
}

func Publish(server *LocalQueue, queue string, r *TestRequest) (err kv.Error) {
	buf, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		return kv.Wrap(errGo).With("request", r.name)
	}
	if err := server.Publish(queue, "", buf); err != nil {
		return err.With("request", r.name)
	}
	return nil
}

func GetExpected(server *LocalQueue, queue string, r *TestRequest) (err kv.Error) {
	msgBytes, msgId, err := server.Get(queue)
	if err != nil {
		return err.With("request", r.name)
	}
	read := TestRequest{}
	errGo := json.Unmarshal(msgBytes, read)
	if errGo != nil {
		return kv.Wrap(errGo).With("request", r.name)
	}
	if read.name != r.name || read.value != r.value {
		return kv.NewError("data mismatch").With("request", r.name).With("name", read.name).With("value", read.value)
	}
	return nil
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
	queue2 := "queue2"
	req1 := TestRequest{
		name: "Iam#1",
		value: 111,
	}

	req2 := TestRequest{
		name: "Iam#2",
		value: 222,
	}

	req3 := TestRequest{
		name: "Iam#3",
		value: 333,
	}

	err := Publish(server, queue1, &req1)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue2, &req1)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue2, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue1, &req2)
	if err != nil {
		t.Fatalf("FAILED to publish #2 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue2, &req2)
	if err != nil {
		t.Fatalf("FAILED to publish #2 to queue %s - %s", queue2, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue1, &req3)
	if err != nil {
		t.Fatalf("FAILED to publish #3 to queue %s - %s", queue1, err.Error())
		return
	}
    time.Sleep(time.Second)
	err = Publish(server, queue2, &req3)
	if err != nil {
		t.Fatalf("FAILED to publish #3 to queue %s - %s", queue2, err.Error())
		return
	}
    time.Sleep(time.Second)

	if err = GetExpected(server, queue1, &req1); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}
	if err = GetExpected(server, queue1, &req2); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}
	if err = GetExpected(server, queue1, &req3); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}

	if err = GetExpected(server, queue2, &req1); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}
	if err = GetExpected(server, queue2, &req2); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}
	if err = GetExpected(server, queue2, &req3); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}

}
