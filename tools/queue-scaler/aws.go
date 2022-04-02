// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package main

import (
	"context"
	"fmt"
	"sort"

	"github.com/andreidenissov-cog/go-service/pkg/server"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/davecgh/go-spew/spew"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"

	"github.com/karlmutch/aws-ec2-price/pkg/price"

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

	if len(cfg.region) == 0 {
		if sess.Config.Region != nil {
			cfg.region = *sess.Config.Region
		}
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
					logger.Trace("heterogenous GPUs unsupported", info.InstanceType, "stack", stack.Trace().TrimRuntime())
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
				// Determine if there are multiple cards and if so divide up the requested slots appropriately
				// before doing a query
				slotsPerCard := uint(status.Resource.Gpus) / uint(*gpuInfo.Count)
				if logger.IsTrace() {
					logger.Trace("Looking at instance ", spew.Sdump(*info.GpuInfo), "slotsPerCard", slotsPerCard, "stack", stack.Trace().TrimRuntime())
				}

				devices, err := cuda.GetDevices(slotsPerCard)
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
				logger.Trace("CPU workload", info.InstanceType, "stack", stack.Trace().TrimRuntime())
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
			if logger.IsTrace() {
				logger.Trace("kept", inst.name, status.Resource.Ram, humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB)*1024*1024), "stack", stack.Trace().TrimRuntime())
			}
			// Build a resource structure to resemble the instance
			// Remove the overhead of the daemonsets etc that we expected, heuristic only
			// Validate it against the job and see if it schedules
			// If not discard it

			// EbsInfo *EbsInfo
			// GpuInfo *GpuInfo
			// InstanceStorageInfo *InstanceStorageInfo
			// MemoryInfo *MemoryInfo
			// ProcessorInfo *ProcessorInfo
			// VCpuInfo *VCpuInfo
			// Remove some cpu and memory as overhead for the daemonsets etc
			availableRam := humanize.Bytes(uint64(*info.MemoryInfo.SizeInMiB-1024) * 1024 * 1024)
			availableCpus := *info.VCpuInfo.DefaultVCpus - 1
			inst.resource = &server.Resource{
				Cpus:     uint(availableCpus),
				Gpus:     0,
				Hdd:      "1024 GB", // HDD is dynamic so we go big when doing matching of resources
				Ram:      availableRam,
				GpuMem:   "0 GB",
				GpuCount: 0,
			}
			if info.GpuInfo != nil {
				if len(info.GpuInfo.Gpus) != 0 {
					if inst.resource.Gpus, err = cuda.GetSlots(*info.GpuInfo.Gpus[0].Manufacturer + " " + *info.GpuInfo.Gpus[0].Name); err != nil {
						logger.Trace("unrecognized GPU present", info.InstanceType, *info.GpuInfo.Gpus[0].Manufacturer+" "+*info.GpuInfo.Gpus[0].Name, "stack", stack.Trace().TrimRuntime())
						continue
					}
					if err != nil {
						logger.Trace(err.Error(), "stack", stack.Trace().TrimRuntime())
						continue
					}
					inst.resource.GpuMem = humanize.Bytes(uint64(*info.GpuInfo.Gpus[0].MemoryInfo.SizeInMiB) * 1024 * 1024)
					inst.resource.GpuCount = uint(*info.GpuInfo.Gpus[0].Count)
				}
			}
			candidates = append(candidates, inst)
		}

		if types.NextToken == nil || len(*types.NextToken) == 0 {
			break
		}
		opts.NextToken = types.NextToken
	}

	logger.Debug("getting pricing, this takes a few moments", "region", cfg.region, "stack", stack.Trace().TrimRuntime())
	pricing, errGo := price.NewPricing(cfg.region)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("region", cfg.region).With("stack", stack.Trace().TrimRuntime())
	}

	instances = []instanceDetails{}

	expensive := [][]string{}
	errors := [][]string{}

	prices := map[string]*price.Instance{}
	detail, errGo := pricing.GetInstances(cfg.region)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("region", cfg.region).With("stack", stack.Trace().TrimRuntime())
	}

	for _, instance := range detail {
		prices[instance.Type] = instance
	}

	logger.Debug("pricing completed", "region", cfg.region, "stack", stack.Trace().TrimRuntime())

	// Now get the least cost machines even if they are much larger than needed
	for _, instance := range candidates {
		detail, isPresent := prices[instance.name]
		if !isPresent {
			errors = append(errors, []string{"instance not priced", "instance", fmt.Sprint(instance), "stack", stack.Trace().TrimRuntime().String()})
			continue
		}
		cost, err := parseMoney(fmt.Sprint(detail.Price))
		if err != nil {
			errors = append(errors, []string{err.Error(), "instance", fmt.Sprint(instance), "stack", stack.Trace().TrimRuntime().String()})
			continue
		}
		isLess, errGo := cfg.maxCost.LessThan(cost)
		if errGo != nil {
			errors = append(errors, []string{errGo.Error(), "instance", fmt.Sprint(instance), "stack", stack.Trace().TrimRuntime().String()})
			continue
		}
		if isLess {
			if logger.IsTrace() {
				expensive = append(expensive, []string{fmt.Sprint(instance), cost.Display()})
			}
			continue
		}
		instance.cost = detail
		instances = append(instances, instance)
	}

	if len(errors) != 0 {
		details := []interface{}{}
		for _, msgItems := range errors {
			for _, msgItem := range msgItems {
				details = append(details, msgItem)
			}
		}
		logger.Warn(details[0].(string), details[1:]...)
	}
	if len(expensive) != 0 {
		details := []interface{}{}
		for _, msgItems := range expensive {
			for _, msgItem := range msgItems {
				details = append(details, msgItem)
			}
		}
		logger.Trace("too expensive", details...)
	}

	sort.Slice(instances, func(i, j int) bool { return instances[i].cost.Price < instances[j].cost.Price })

	return instances, nil
}
