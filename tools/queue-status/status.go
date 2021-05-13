// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"github.com/leaf-ai/go-service/pkg/server"
)

// The file contains the implementation of a queue state data structure
type Queues map[string]QStatus

type QStatus struct {
	name       string
	Ready      int
	NotVisible int
	Resource   *server.Resource `json:"Resource,omitempty"`
}
