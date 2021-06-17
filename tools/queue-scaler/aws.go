// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"

	"github.com/odg0318/aws-ec2-price/pkg/price"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

const (
	MaxResults = int64(100)
)

// NewSession invokes the AWS API NewSession using the configuration structure this command
// uses
func NewSession(ctx context.Context, cfg *Config) (sess *session.Session, err kv.Error) {

	opts := session.Options{
		SharedConfigState: session.SharedConfigEnable,
	}
	if len(cfg.accessKey) != 0 || len(cfg.secretKey) != 0 {
		if len(cfg.accessKey) == 0 {
			return nil, kv.NewError("secret key specified but access key was not specified").With("stack", stack.Trace().TrimRuntime())
		}
		if len(cfg.secretKey) == 0 {
			return nil, kv.NewError("secret key not specified but access key was specified").With("stack", stack.Trace().TrimRuntime())
		}
		if len(cfg.region) == 0 {
			return nil, kv.NewError("region needs to be supplied when keys are specified").With("stack", stack.Trace().TrimRuntime())
		}
		opts = session.Options{
			Config: aws.Config{
				Credentials: credentials.NewStaticCredentials(cfg.accessKey, cfg.secretKey, ""),
				Region:      &cfg.region,
			},
		}
	}

	sess, errGo := session.NewSessionWithOptions(opts)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return sess, nil
}

// ec2Instances gets the cheapest machines that satisfy the conditions specified as inputs in the status
// parameter
//
func ec2Instances(ctx context.Context, cfg *Config, sess *session.Session, status *QStatus) (instances []instanceDetails, err kv.Error) {

	svc := ec2.New(sess)

	maxResults := MaxResults
	opts := ec2.DescribeInstanceTypesInput{
		MaxResults: &maxResults,
	}

	candidates := []instanceDetails{}

	// First go through excluding the instances that simply dont have enough resources.
	for {
		types, errGo := svc.DescribeInstanceTypes(&opts)
		if errGo != nil {
			return instances, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		for _, info := range types.InstanceTypes {
			if info.InstanceType == nil || len(*info.InstanceType) == 0 {
				continue
			}

			inst := instanceDetails{
				name: *info.InstanceType,
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

				if len(info.GpuInfo.Gpus) != 1 {
					logger.Trace("homogenous GPUs unsupported", info.InstanceType, "stack", stack.Trace().TrimRuntime())
					continue
				}
				gpuInfo := *info.GpuInfo.Gpus[0]

				if gpuInfo.MemoryInfo == nil || gpuInfo.MemoryInfo.SizeInMiB == nil {
					logger.Trace("GPUs memory size was unsupported", info.InstanceType, "stack", stack.Trace().TrimRuntime())
					continue
				}

				memNeededMiB := int64(mem / 1024 / 1024)
				if *gpuInfo.MemoryInfo.SizeInMiB < memNeededMiB {
					logger.Trace("insufficent GPU mem", info.InstanceType, status.Resource.GpuMem,
						humanize.Bytes(uint64(*gpuInfo.MemoryInfo.SizeInMiB)*1024*1024), "stack", stack.Trace().TrimRuntime())
					continue
				}
				devices, err := cuda.GetDevices(status.Resource.Gpus)
				if err != nil {
					logger.Trace(err.Error(), "stack", stack.Trace().TrimRuntime())
					continue
				}
				devName := *gpuInfo.Manufacturer + " " + *gpuInfo.Name

				found := false
				for _, device := range devices {
					if devName == device {
						found = true
						break
					}
				}
				if !found {
					logger.Trace("GPU not supported", info.InstanceType, devName, "stack", stack.Trace().TrimRuntime())
					continue
				}

				// Having obtained the device name we can now search out DB of know cards and make sure
				// this specific machine type has the capacity
			} else {
				if info.GpuInfo != nil {
					// Dont waste GPU instances on non GPU activities
					logger.Trace("GPU not needed", info.InstanceType, "stack", stack.Trace().TrimRuntime())
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
			logger.Trace("kept", inst.name, status.Resource.Ram, humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB)*1024*1024))

			// EbsInfo *EbsInfo
			// GpuInfo *GpuInfo
			// InstanceStorageInfo *InstanceStorageInfo
			// MemoryInfo *MemoryInfo
			// ProcessorInfo *ProcessorInfo
			// VCpuInfo *VCpuInfo
			inst.resource = &server.Resource{
				Cpus:   uint(*info.VCpuInfo.DefaultVCpus),
				Gpus:   0,
				Hdd:    "1024Gb", // HDD is dynamic so we go big when doing matching of resources
				Ram:    humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB) * 1024 * 1024),
				GpuMem: "0Gb",
			}
			if info.GpuInfo != nil {
				if len(info.GpuInfo.Gpus) != 0 {
					if inst.resource.Gpus, err = cuda.GetSlots(*info.GpuInfo.Gpus[0].Manufacturer + " " + *info.GpuInfo.Gpus[0].Name); err != nil {
						logger.Trace("unrecognized GPU present", info.InstanceType, *info.GpuInfo.Gpus[0].Manufacturer+" "+*info.GpuInfo.Gpus[0].Name)
						continue
					}
					if err != nil {
						logger.Trace(err.Error(), "stack", stack.Trace().TrimRuntime())
						continue
					}
					inst.resource.GpuMem = humanize.Bytes(uint64(*info.GpuInfo.Gpus[0].MemoryInfo.SizeInMiB * 1024 * 1024))
				}
			}
			candidates = append(candidates, inst)
		}

		if types.NextToken == nil || len(*types.NextToken) == 0 {
			break
		}
		opts.NextToken = types.NextToken
	}

	logger.Debug("getting pricing, this takes a few moments", "stack", stack.Trace().TrimRuntime())
	pricing, errGo := price.NewPricing()
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	instances = []instanceDetails{}

	// Now get the least cost machines even if they are much larger than needed
	for _, instance := range candidates {
		detail, errGo := pricing.GetInstance(cfg.region, instance.name)
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
		instance.cost = detail
		instances = append(instances, instance)
	}

	sort.Slice(instances, func(i, j int) bool { return instances[i].cost.Price < instances[j].cost.Price })

	return instances, nil
}
