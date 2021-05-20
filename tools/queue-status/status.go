// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/odg0318/aws-ec2-price/pkg/price"
)

// The file contains the implementation of a queue state data structure
type Queues map[string]QStatus

type Instance struct {
	Name        string
	GpuCount    int
	SlotsPerGpu int
}

type QStatus struct {
	name       string            // The logical queue name
	Ready      int               // The approximate number of messages that are waiting for runners
	NotVisible int               // The approximate number of messages currently claimed by runners
	Resource   *server.Resource  `json:"Resource,omitempty"`     // The hardware resources needed by peeking at the first request in the queue
	Instances  []*price.Instance `json:"AWSInstances,omitempty"` // AWS instance types that could fit this queues requests
	NodeGroup  string            `json:"NodeGroup,omitempty"`    // An identified node group that can be used, if found
}
