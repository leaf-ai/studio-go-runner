// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// Unit tests for FileQueue implementation.

import (
	"testing"
)

func TestFileQueue(t *testing.T) {

	queue, err := NewFileQueue("/home/ubuntu/qpoint", "q1", nil, nil)
	if err != nil {
		t.Fail()
	}






}
