// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for LocalQueue implementation.

import (
	"context"
	"encoding/json"
	"github.com/jjeffery/kv" // MIT License
	"os"
	"path"
	"testing"

	"github.com/leaf-ai/go-service/pkg/log"
)

type TestRequest struct {
	Name  string `json:"Name"`
	Value int    `json:"Value"`
}

func publish(server *LocalQueue, queue string, r *TestRequest) (err kv.Error) {
	buf, errGo := json.MarshalIndent(r, "", "  ")
	if errGo != nil {
		return kv.Wrap(errGo).With("request", r.Name)
	}
	if err := server.Publish(queue, "application/json", buf, true); err != nil {
		return err.With("request", r.Name)
	}
	return nil
}

func getExpected(server *LocalQueue, queue string, rmap *map[string]int) (err kv.Error) {
	queuePath := path.Join(server.RootDir, queue)
	msgBytes, _, err := server.Get(queuePath)
	if err != nil {
		return err.With("queue", queue)
	}
	read := &TestRequest{}
	errGo := json.Unmarshal(msgBytes, read)
	if errGo != nil {
		return kv.Wrap(errGo).With("queue", queue)
	}

	value, ok := (*rmap)[read.Name]
	if !ok {
		return kv.NewError("key not found").With("request", read.Name)
	}
	if value != read.Value {
		return kv.NewError("data mismatch").With("request", read.Name).With("Value", read.Value).With("Expected", value)
	}
	delete(*rmap, read.Name)
	return nil
}

func verifyEmpty(server *LocalQueue, queue string) (err kv.Error) {
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

	err := publish(server, queue1, &req1)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue1, err.Error())
		return
	}
	err = publish(server, queue2, &req1)
	if err != nil {
		t.Fatalf("FAILED to publish #1 to queue %s - %s", queue2, err.Error())
		return
	}
	err = publish(server, queue1, &req2)
	if err != nil {
		t.Fatalf("FAILED to publish #2 to queue %s - %s", queue1, err.Error())
		return
	}
	err = publish(server, queue2, &req2)
	if err != nil {
		t.Fatalf("FAILED to publish #2 to queue %s - %s", queue2, err.Error())
		return
	}
	err = publish(server, queue1, &req3)
	if err != nil {
		t.Fatalf("FAILED to publish #3 to queue %s - %s", queue1, err.Error())
		return
	}
	err = publish(server, queue2, &req3)
	if err != nil {
		t.Fatalf("FAILED to publish #3 to queue %s - %s", queue2, err.Error())
		return
	}

	mapQueue1 := make(map[string]int)
	mapQueue1[req1.Name] = req1.Value
	mapQueue1[req2.Name] = req2.Value
	mapQueue1[req3.Name] = req3.Value
	mapQueue2 := make(map[string]int)
	mapQueue2[req1.Name] = req1.Value
	mapQueue2[req2.Name] = req2.Value
	mapQueue2[req3.Name] = req3.Value

	if err = getExpected(server, queue1, &mapQueue1); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}
	if err = getExpected(server, queue1, &mapQueue1); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}
	if err = getExpected(server, queue1, &mapQueue1); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue1, err.Error())
		return
	}
	if err = getExpected(server, queue2, &mapQueue2); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}
	if err = getExpected(server, queue2, &mapQueue2); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}
	if err = getExpected(server, queue2, &mapQueue2); err != nil {
		t.Fatalf("READ BACK data error: queue %s - %s", queue2, err.Error())
		return
	}
	if len(mapQueue1) != 0 {
		t.Fatalf("REQUESTS LEFT UNREAD: queue %s - %v", queue1, mapQueue1)
	}
	if len(mapQueue2) != 0 {
		t.Fatalf("REQUESTS LEFT UNREAD: queue %s - %v", queue2, mapQueue2)
	}
	if err = verifyEmpty(server, queue1); err != nil {
		t.Fatalf("VERIFY QUEUE is empty: queue %s - %s", queue1, err.Error())
		return
	}
	if err = verifyEmpty(server, queue2); err != nil {
		t.Fatalf("VERIFY QUEUE is empty: queue %s - %s", queue2, err.Error())
		return
	}
}
