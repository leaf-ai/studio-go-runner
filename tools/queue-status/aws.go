// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/odg0318/aws-ec2-price/pkg/price"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

const (
	MaxResults = int64(100)
)

// ec2Instances gets the cheapest machines that satisfy the conditions specified as inputs in the status
// parameter
//
func ec2Instances(ctx context.Context, cfg *Config, sess *session.Session, status *QStatus) (costs []*price.Instance, err kv.Error) {

	svc := ec2.New(sess)

	maxResults := MaxResults
	opts := ec2.DescribeInstanceTypesInput{
		MaxResults: &maxResults,
	}

	instances := []string{}

	// First go through excluding the instances that simply dont have enough resources.
	for {
		types, errGo := svc.DescribeInstanceTypes(&opts)
		if errGo != nil {
			return costs, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		for _, info := range types.InstanceTypes {
			if info.InstanceType == nil || len(*info.InstanceType) == 0 {
				continue
			}

			inst := Instance{
				Name: *info.InstanceType,
			}

			// Get the slots value for the resource
			if status.Resource.Gpus != 0 {
				if info.GpuInfo == nil {
					continue
				}
				mem, errGo := humanize.ParseBytes(status.Resource.GpuMem)
				if errGo != nil {
					logger.Warn("Invalid GPU RAM amount", "Queue", status.name, "GPU RAM value", status.Resource.GpuMem, "stack", stack.Trace().TrimRuntime())
					continue
				}

				memNeededMiB := int64(mem / 1024 / 1024)
				if *info.GpuInfo.TotalGpuMemoryInMiB < memNeededMiB {
					logger.Trace("insufficent GPU mem", info.InstanceType, status.Resource.GpuMem, humanize.Bytes(uint64(*info.GpuInfo.TotalGpuMemoryInMiB)*1024*1024))
					continue
				}
			} else {
				if info.GpuInfo != nil {
					// Dont waste GPU instances on non GPU activities
					logger.Trace("GPU not needed", info.InstanceType)
					continue
				}
			}

			// Check the machine RAM ensure we dont get a machine too small or large
			if len(status.Resource.Ram) != 0 {
				ram, errGo := humanize.ParseBytes(status.Resource.Ram)
				if errGo != nil {
					logger.Warn("Invalid RAM amount", "Queue", status.name, "RAM value", status.Resource.Ram, "stack", stack.Trace().TrimRuntime())
					continue
				}
				// When we check for RAM make sure we have about 512MiB for overhead
				machineRam := uint64(*info.MemoryInfo.SizeInMiB)
				ramNeededMiB := uint64(ram/1024/1024 + 512)

				if ramNeededMiB > machineRam {
					logger.Trace("ram too small", info.InstanceType, status.Resource.Ram, humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB)*1024*1024))
					continue
				}
			}
			logger.Trace("kept", inst.Name, status.Resource.Ram, humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB)*1024*1024))

			// EbsInfo *EbsInfo
			// GpuInfo *GpuInfo
			// InstanceStorageInfo *InstanceStorageInfo
			// MemoryInfo *MemoryInfo
			// ProcessorInfo *ProcessorInfo
			// VCpuInfo *VCpuInfo
			instances = append(instances, inst.Name)
		}

		if types.NextToken == nil || len(*types.NextToken) == 0 {
			break
		}
		opts.NextToken = types.NextToken
	}

	logger.Debug("getting pricing, this takes a few moments", "stack", stack.Trace().TrimRuntime())
	pricing, errGo := price.NewPricing()
	if errGo != nil {
		return costs, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	costs = []*price.Instance{}

	// Now get the least cost machines even if they are much larger than needed
	for _, instance := range instances {
		detail, errGo := pricing.GetInstance("us-west-2", instance)
		if errGo != nil {
			logger.Trace(errGo.Error(), "instance", instance, "stack", stack.Trace().TrimRuntime())
			continue
		}
		cost, errGo := parseMoney(fmt.Sprint(detail.Price))
		if errGo != nil {
			logger.Trace(errGo.Error(), "instance", instance, "stack", stack.Trace().TrimRuntime())
			continue
		}
		isLess, errGo := cfg.maxCost.LessThan(cost)
		if errGo != nil {
			logger.Trace(errGo.Error(), "instance", instance, "stack", stack.Trace().TrimRuntime())
			continue
		}
		if isLess {
			logger.Trace("too expensive", instance, cost.Display())
			continue
		}
		costs = append(costs, detail)
	}

	sort.Slice(costs, func(i, j int) bool { return costs[i].Price < costs[j].Price })

	return costs, nil
}
