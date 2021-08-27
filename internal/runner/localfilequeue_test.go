// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for LocalQueue implementation.

import (
	"context"
	"encoding/json"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"os"
	"path"
	"testing"
	"time"

	"github.com/leaf-ai/go-service/pkg/log"
)

type TestRequest struct {
	Name  string `json:"Name"`
	Value int    `json:"Value"`
}

func Publish(server *LocalQueue, queue string, r *TestRequest) (err kv.Error) {
	buf, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		return kv.Wrap(errGo).With("request", r.Name)
	}
	if err := server.Publish(queue, "application/json", buf); err != nil {
		return err.With("request", r.Name)
	}
	return nil
}

func GetExpected(server *LocalQueue, queue string, r *TestRequest) (err kv.Error) {
	queuePath := path.Join(server.RootDir, queue)
	msgBytes, _, err := server.Get(queuePath)
	if err != nil {
		return err.With("request", r.Name)
	}
	read := &TestRequest{}
	errGo := json.Unmarshal(msgBytes, read)
	if errGo != nil {
		return kv.Wrap(errGo).With("request", r.Name)
	}
	if read.Name != r.Name || read.Value != r.Value {
		return kv.NewError("data mismatch").With("request", r.Name).With("Name", read.Name).With("Value", read.Value)
	}
	return nil
}

func VerifyEmpty(server *LocalQueue, queue string) (err kv.Error) {
	queuePath := path.Join(server.RootDir, queue)
	hasWork, err := server.HasWork(context.Background(), queuePath)
	if err != nil {
		return err.With("queue", queue)
	}
	if hasWork {
		return kv.NewError("not empty").With("queue", queue)
	}
	return nil
}

func showModTimes(server *LocalQueue, queue string, t *testing.T) (err kv.Error) {
	queuePath := path.Join(server.RootDir, queue)
	root, errGo := os.Open(queuePath)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	defer root.Close()

	listInfo, errGo := root.Readdir(-1)
	if errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("path", queuePath)
	}
	t.Log("Dir: ", queuePath)
	for _, info := range listInfo {
		t.Log(info.Name(), "modTime", info.ModTime())
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
		Name:  "Iam#1",
		Value: 111,
	}

	req2 := TestRequest{
		Name:  "Iam#2",
		Value: 222,
	}

	req3 := TestRequest{
		Name:  "Iam#3",
		Value: 333,
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

	showModTimes(server, queue1, t)
	showModTimes(server, queue2, t)

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
	if err = VerifyEmpty(server, queue1); err != nil {
		t.Fatalf("VERIFY QUEUE is empty: queue %s - %s", queue1, err.Error())
		return
	}
	if err = VerifyEmpty(server, queue2); err != nil {
		t.Fatalf("VERIFY QUEUE is empty: queue %s - %s", queue2, err.Error())
		return
	}
}
