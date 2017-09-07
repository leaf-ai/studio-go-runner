package runner

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"fmt"
	"strings"
)

type CpuAllocated struct {
	slots uint
	mem   uint64
}

type DiskAllocated struct {
	device string
	size   uint64
}

type Allocated struct {
	group string
	gpu   *GpuAllocated
	cpu   *CPUAllocated
	disk  *DiskAllocated
}

type AllocRequest struct {
	Group     string // Used to cluster together requests that can share some types of partitioned resources
	MaxCPU    uint
	MaxMem    uint64
	MaxGPU    uint
	MaxGPUMem uint64
	MaxDisk   uint64
}

type Resources struct{}

// NewResources is used to get a receiver for dealing with the
// resources being tracked by the studioml runner
//
func NewResources(localDisk string) (rsc *Resources, err error) {

	err = initDiskResource(localDisk)

	return &Resources{}, err
}

// Dump is used by monitoring components to obtain a debugging style dump of
// the resources and their currently allocated state
//
func (*Resources) Dump() (dump string) {

	items := []string{}

	if item := DumpCPU(); len(item) > 0 {
		items = append(items, item)
	}

	if item := DumpGPU(); len(item) > 0 {
		items = append(items, item)
	}

	if item := DumpDisk(); len(item) > 0 {
		items = append(items, item)
	}

	return fmt.Sprintf("[%s]", strings.Join(items, ", "))
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
func (*Resources) AllocResources(rqst AllocRequest) (alloc *Allocated, err error) {

	alloc = &Allocated{}

	// Allocate the GPU resources first
	if alloc.gpu, err = AllocGPU(rqst.Group, rqst.MaxGPU, rqst.MaxGPUMem); err != nil {
		return nil, err
	}

	// CPU resources next
	if alloc.cpu, err = AllocCPU(rqst.MaxCPU, rqst.MaxMem); err != nil {
		alloc.Release()
		return nil, err
	}

	// Lastly, disk storage
	if alloc.disk, err = AllocDisk(rqst.MaxDisk); err != nil {
		alloc.Release()
		return nil, err
	}

	alloc.group = rqst.Group
	return alloc, nil
}

// Return any allocated resources to the sub system from which they were obtained
//
func (a *Allocated) Release() (errs []error) {

	errs = []error{}

	if a == nil {
		return []error{fmt.Errorf("unexpected nil supplied for the release of resources")}
	}

	if a.gpu != nil {
		if e := ReturnGPU(a.gpu); e != nil {
			errs = append(errs, e)
		}
	}

	if a.cpu != nil {
		a.cpu.Release()
	}

	if a.disk != nil {
		if err := a.disk.Release(); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}
