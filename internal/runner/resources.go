package runner

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
)

// CpuAllocated hold information about cpu and memory being used
type CpuAllocated struct {
	slots uint
	mem   uint64
}

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

// Receiver for resource related methods
//
type Resources struct{}

// NewResources is used to get a receiver for dealing with the
// resources being tracked by the studioml runner
//
func NewResources(localDisk string) (rsc *Resources, err errors.Error) {

	err = initDiskResource(localDisk)

	return &Resources{}, err
}

// AllocResources will go through all requested resources and allocate them using the resource APIs.
//
// If any single resource be not available then the ones done so far will be released.  The use of a receiver
// pointer is to make sure that the caller invokes the NewResources to populate some of the allocators with the
// context they require to track consumption of some types of resources, such as selecting the disk from which
// allocations will be performed.
//
// The caller is responsible for calling the release method when the resources are no longer needed.
//
func (*Resources) AllocResources(rqst AllocRequest) (alloc *Allocated, err errors.Error) {

	alloc = &Allocated{}

	// Allocate the GPU resources first, they are typically the least available
	if alloc.GPU, err = AllocGPU(rqst.MaxGPU, rqst.MaxGPUMem, rqst.GPUDivisibles); err != nil {
		return nil, err
	}

	// CPU resources next
	if alloc.CPU, err = AllocCPU(rqst.MaxCPU, rqst.MaxMem); err != nil {
		alloc.Release()
		return nil, err
	}

	// Lastly, disk storage
	if alloc.Disk, err = AllocDisk(rqst.MaxDisk); err != nil {
		alloc.Release()
		return nil, err
	}

	return alloc, nil
}

// Release returns any allocated resources to the sub system from which they were obtained
//
func (a *Allocated) Release() (errs []errors.Error) {

	errs = []errors.Error{}

	if a == nil {
		return []errors.Error{errors.New("unexpected nil supplied for the release of resources").With("stack", stack.Trace().TrimRuntime())}
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
	}

	return errs
}
