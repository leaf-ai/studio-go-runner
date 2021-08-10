// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for FileQueue implementation.

import (
	"testing"
	"fmt"

	"github.com/leaf-ai/studio-go-runner/internal/runner"
)

func TestFileQueue(t *testing.T) {
	fmt.Println("Hello world!")

	queue, err := NewFileQueue("/home/ubuntu/qpoint", "q1", nil, nil)
	if err != nil {
		t.Fail()
	}


	queue, err := NewFileQueue("/home/ubuntu/qpoint", "q1", nil, nil)
	if err != nil {
		t.Fail()
	}






}
