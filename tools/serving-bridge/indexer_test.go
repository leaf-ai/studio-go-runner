// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"testing"
	"time"

	"github.com/leaf-ai/studio-go-runner/pkg/log"
)

func TestIndexer(t *testing.T) {
	logger := log.NewLogger("test-indexer")
	defer logger.Warn("completed")

	time.Sleep(10 * time.Second)
}
