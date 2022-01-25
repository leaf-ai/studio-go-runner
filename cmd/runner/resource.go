package main

import (
	"errors"
	"sync"

	"github.com/leaf-ai/go-service/pkg/server"
	"github.com/leaf-ai/studio-go-runner/internal/resources"
	"github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/dustin/go-humanize"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	// Used to prevent multiple threads doing resource allocations for debugging and auditing purposes
	guardAllocation sync.Mutex
)

// This file contains the implementation of resource handling functions
// used by the processor.  It can be used in a mode, using live, as a means
// of checking for resources without actually allocation them.

func allocResource(rsc *server.Resource, id string, live bool) (alloc *resources.Allocated, err kv.Error) {
	if rsc == nil {
		return nil, kv.NewError("resource missing").With("stack", stack.Trace().TrimRuntime())
	}
	rqst := resources.AllocRequest{}

	// Before continuing locate GPU resources for the task that has been received
	//
	errGo := errors.New("")
	// The GPU values are optional and default to 0
	if 0 != len(rsc.GpuMem) {
		if rqst.MaxGPUMem, errGo = runner.ParseBytes(rsc.GpuMem); errGo != nil {
			// TODO Add an output function here for Issues #4, https://github.com/leaf-ai/studio-go-runner/issues/4
			return nil, kv.Wrap(errGo, "gpuMem value is invalid").With("gpuMem", rsc.GpuMem).With("stack", stack.Trace().TrimRuntime())
		}
	}

	rqst.MaxGPU = uint(rsc.Gpus)
	// ASD HACK: rqst.MaxGPUCount = int(rsc.GpuCount)
	rqst.MaxGPUCount = int(rsc.Gpus)
	// ASD HACK: rqst.GPUUnits = []int{int(rsc.Gpus)}
	rqst.GPUUnits = []int{int(rsc.GpuCount)}

	rqst.MaxCPU = uint(rsc.Cpus)
	if rqst.MaxMem, errGo = humanize.ParseBytes(rsc.Ram); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	if rqst.MaxDisk, errGo = humanize.ParseBytes(rsc.Hdd); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	guardAllocation.Lock()
	defer guardAllocation.Unlock()

	machineRcs := (&resources.Resources{}).FetchMachineResources()

	logables := []interface{}{"experiment_id", id, "before", machineRcs.String()}

	if alloc, err = machineResources.Alloc(rqst, live); err != nil {
		return nil, err
	}

	logables = append(logables, rqst.Logable()...)
	if live {
		logables = append(logables, alloc.Logable()...)
		machineRcs = (&resources.Resources{}).FetchMachineResources()
		logables = append(logables, "after", machineRcs.String())
	}
	logger.Debug("alloc done", logables...)

	return alloc, nil
}

func deallocResource(alloc *resources.Allocated, id string) {
	guardAllocation.Lock()
	defer guardAllocation.Unlock()

	machineRcs := (&resources.Resources{}).FetchMachineResources()

	logables := []interface{}{"experiment_id", id, "before", machineRcs.String()}

	if errs := alloc.Release(); len(errs) != 0 {
		for _, err := range errs {
			logger.Warn("alloc not released", kv.Wrap(err).With(logables...))
		}
	} else {
		machineRcs = (&resources.Resources{}).FetchMachineResources()
		logables = append(logables, "after", machineRcs.String())
		logger.Debug("alloc released", logables...)
	}
}
