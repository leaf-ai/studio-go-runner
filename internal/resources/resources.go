// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package resources

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"strconv"

	"github.com/leaf-ai/go-service/pkg/server"

	"github.com/leaf-ai/studio-go-runner/internal/cpu_resource"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"
	"github.com/leaf-ai/studio-go-runner/internal/disk_resource"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// Allocated gathers together data for allocations of machine level resources
// into a single data structure that can be used to track resource allocations for
// tasks
//
type Allocated struct {
	GPU  cuda.GPUAllocations
	CPU  *cpu_resource.CPUAllocated
	Disk *disk_resource.DiskAllocated
}

func (alloc *Allocated) Logable() (logable []interface{}) {
	logable = []interface{}{"allocated_CPU", alloc.CPU.Cores, "allocated_cpu_mem", humanize.Bytes(alloc.CPU.Mem),
		"allocated_disk", humanize.Bytes(alloc.Disk.Size)}
	for i, aGPU := range alloc.GPU {
		logable = append(logable, "allocated_GPU "+strconv.Itoa(i)+"_slots", aGPU.Slots, "allocated_GPU"+strconv.Itoa(i)+"_mem", humanize.Bytes(aGPU.Mem))
	}
	return logable
}

// AllocRequest is used by clients to make requests for specific types of machine resources
//
type AllocRequest struct {
	MaxCPU        uint
	MaxMem        uint64
	MaxGPU        uint   // GPUs are allocated using slots which approximate their throughput
	GPUDivisibles []uint // The small quantity of slots that are permitted for allocation for when multiple cards must be used
	MaxGPUMem     uint64
	MaxDisk       uint64
}

func (rqst *AllocRequest) Logable() (logable []interface{}) {
	return []interface{}{"request_CPU", rqst.MaxCPU, "request_GPU_mem", humanize.Bytes(rqst.MaxMem),
		"request_GPU", rqst.MaxGPU, "request_GPU_mem", humanize.Bytes(rqst.MaxGPUMem),
		"request_disk", humanize.Bytes(rqst.MaxDisk)}
}

// Resources is a receiver for resource related methods used to describe execution requirements
//
type Resources struct{}

// FetchMachineResources extracts the current system state in terms of memory etc
// and coverts this into the resource specification used by jobs.  Because resources
// specified by users are not exact quantities the resource is used for the machines
// resources even in the face of some loss of precision
//
func (*Resources) FetchMachineResources() (rsc *server.Resource) {

	rsc = &server.Resource{}

	// For specified queue look for any free slots on existing GPUs is
	// applicable and fill them, or find empty GPUs and groups to fill
	// in with work

	cpus, v := cpu_resource.CPUFree()
	rsc.Cpus = uint(cpus)
	rsc.Ram = humanize.Bytes(v)

	rsc.Hdd = humanize.Bytes(disk_resource.GetDiskFree())

	// go runner allows GPU resources at the board level so obtain the total slots across
	// all board form factors and use that as our max
	//
	rsc.Gpus = cuda.TotalFreeGPUSlots()
	rsc.GpuMem = humanize.Bytes(cuda.LargestFreeGPUMem())

	return rsc
}

// NewResources is used to get a receiver for dealing with the
// resources being tracked by the studioml runner
//
func NewResources(localDisk string) (rsc *Resources, err kv.Error) {

	err = disk_resource.InitDiskResource(localDisk)

	return &Resources{}, err
}

// Alloc will go through all requested resources and allocate them using the resource APIs.
//
// If any single resource be not available then the ones done so far will be released.  The use of a receiver
// pointer is to make sure that the caller invokes the NewResources to populate some of the allocators with the
// context they require to track consumption of some types of resources, such as selecting the disk from which
// allocations will be performed.
//
// The caller is responsible for calling the release method when the resources are no longer needed.
//
// The live parameter can be used to controller whether the allocation attempts will perform
// an allocation (true), or whether they will simply test (false) that the allocation would have been
// completed successfully.
//
func (*Resources) Alloc(rqst AllocRequest, live bool) (alloc *Allocated, err kv.Error) {

	alloc = &Allocated{}

	// Each of the resources being allocated contain code to lock the individual resources
	// the deallocation handles the release on a per resource basis

	// Allocate the GPU resources first, they are typically the least available
	if alloc.GPU, err = cuda.AllocGPU(rqst.MaxGPU, rqst.MaxGPUMem, rqst.GPUDivisibles, live); err != nil {
		return nil, err
	}

	// CPU resources next
	if alloc.CPU, err = cpu_resource.AllocCPU(rqst.MaxCPU, rqst.MaxMem, live); err != nil {
		if live {
			alloc.Release()
		}
		return nil, err
	}

	// Lastly, disk storage
	if alloc.Disk, err = disk_resource.AllocDisk(rqst.MaxDisk, live); err != nil {
		if live {
			alloc.Release()
		}
		return nil, err
	}

	return alloc, nil
}

// Release returns any allocated resources to the sub system from which they were obtained
//
func (a *Allocated) Release() (errs []kv.Error) {

	errs = []kv.Error{}

	if a == nil {
		return []kv.Error{kv.NewError("unexpected nil supplied for the release of resources").With("stack", stack.Trace().TrimRuntime())}
	}

	for _, gpuAlloc := range a.GPU {
		if e := cuda.ReturnGPU(gpuAlloc); e != nil {
			errs = append(errs, e)
		}
	}

	if a.CPU != nil {
		a.CPU.Release()
	}

	if a.Disk != nil {
		if err := a.Disk.Release(); err != nil {
			errs = append(errs, err)
		}
	} else {
		errs = append(errs, kv.NewError("disk block missing").With("stack", stack.Trace().TrimRuntime()))
	}

	return errs
}
