// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/karlmutch/aws-ec2-price/pkg/price"
	"github.com/leaf-ai/go-service/pkg/server"
)

// The file contains the implementation of a queue state data structure
type Queues map[string]QStatus

type instanceDetails struct {
	name     string
	cost     *price.Instance
	info     *ec2.InstanceTypeInfo
	resource *server.Resource
}

type Instance struct {
	Name        string
	GpuCount    int
	SlotsPerGpu int
}

type QStatus struct {
	name       string                         // The logical queue name
	Ready      int                            // The approximate number of messages that are waiting for runners
	NotVisible int                            // The approximate number of messages currently claimed by runners
	Running    int                            // The number of known pods that are alive and running work
	Resource   *server.Resource               `json:"Resource,omitempty"`     // The hardware resources needed by peeking at the first request in the queue
	Instances  []instanceDetails              `json:"AWSInstances,omitempty"` // AWS instance types that could fit this queues requests
	NodeGroup  string                         `json:"NodeGroup,omitempty"`    // An identified node group that can be used, if found
	Jobs       map[string]map[string]struct{} `json:"Jobs,omitempty"`         // The known jobs that exist within the cluster, job-name major, containing a slice of pod-names
}
