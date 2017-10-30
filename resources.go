package runner

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"fmt"
	"strings"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
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
	GPU   *GPUAllocated
	CPU   *CPUAllocated
	Disk  *DiskAllocated
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
func NewResources(localDisk string) (rsc *Resources, err errors.Error) {

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
func (*Resources) AllocResources(rqst AllocRequest) (alloc *Allocated, err errors.Error) {

	alloc = &Allocated{}

	// Allocate the GPU resources first
	if alloc.GPU, err = AllocGPU(rqst.Group, rqst.MaxGPU, rqst.MaxGPUMem); err != nil {
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

	alloc.group = rqst.Group
	return alloc, nil
}

// Return any allocated resources to the sub system from which they were obtained
//
func (a *Allocated) Release() (errs []errors.Error) {

	errs = []errors.Error{}

	if a == nil {
		return []errors.Error{errors.New("unexpected nil supplied for the release of resources").With("stack", stack.Trace().TrimRuntime())}
	}

	if a.GPU != nil {
		if e := ReturnGPU(a.GPU); e != nil {
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
