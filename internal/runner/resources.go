// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"strconv"

	humanize "github.com/dustin/go-humanize"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

// DiskAllocated hold information about disk resources consumed on a specific device
type DiskAllocated struct {
	device string
	size   uint64
}

// Allocated gathers together data for allocations of machine level resources
// into a single data structure that can be used to track resource allocations for
// tasks
//
type Allocated struct {
	GPU  GPUAllocations
	CPU  *CPUAllocated
	Disk *DiskAllocated
}

func (alloc *Allocated) Logable() (logable []interface{}) {
	logable = []interface{}{"allocated CPU", alloc.CPU.cores, "allocated cpu mem", humanize.Bytes(alloc.CPU.mem),
		"allocated disk", humanize.Bytes(alloc.Disk.size)}
	for i, aGPU := range alloc.GPU {
		logable = append(logable, "allocated GPU "+strconv.Itoa(i), aGPU.slots, "allocated gpu mem "+strconv.Itoa(i), humanize.Bytes(aGPU.mem))
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
	return []interface{}{"request CPU", rqst.MaxCPU, "request cpu mem", humanize.Bytes(rqst.MaxMem),
		"request GPU", rqst.MaxGPU, "request gpu mem", humanize.Bytes(rqst.MaxGPUMem),
		"request disk", humanize.Bytes(rqst.MaxDisk)}
}

// Resources is a receiver for resource related methods used to describe execution requirements
//
type Resources struct{}

// NewResources is used to get a receiver for dealing with the
// resources being tracked by the studioml runner
//
func NewResources(localDisk string) (rsc *Resources, err kv.Error) {

	err = initDiskResource(localDisk)

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
	if alloc.GPU, err = AllocGPU(rqst.MaxGPU, rqst.MaxGPUMem, rqst.GPUDivisibles, live); err != nil {
		return nil, err
	}

	// CPU resources next
	if alloc.CPU, err = AllocCPU(rqst.MaxCPU, rqst.MaxMem, live); err != nil {
		if live {
			alloc.Release()
		}
		return nil, err
	}

	// Lastly, disk storage
	if alloc.Disk, err = AllocDisk(rqst.MaxDisk, live); err != nil {
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
		if e := ReturnGPU(gpuAlloc); e != nil {
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
