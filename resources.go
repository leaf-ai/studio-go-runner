package runner

// This file contains the implementation of a resource tracker for a local host on
// which CUDA, storage, and main motherboard resources can be found and tracked on
// behalf of an application

import (
	"fmt"
)

type CpuAllocated struct {
	slots uint
	mem   uint64
}

type DiskAllocated struct {
	size uint64
}

type Allocated struct {
	proj string
	gpu  *GpuAllocated
	cpu  *CPUAllocated
	disk *DiskAllocated
}

type AllocRequest struct {
	Proj      string
	MaxCPU    uint32
	MaxMem    uint64
	MaxGPU    uint
	MaxGPUMem uint64
}

// AllocResources will go through all requested resources and allocate them using the resource APIs
//
// If any single resource be not available then the ones done so far will be released.
//
// The caller is responsible for calling the release method when the resources are no longer needed.
//
func AllocResources(rqst AllocRequest) (alloc *Allocated, err error) {

	a := &Allocated{}

	defer func() {
		if a != nil {
			a.Release()
		}
	}()

	// Allocate the GPU resources first
	if a.gpu, err = AllocGPU(rqst.Proj, rqst.MaxGPU, rqst.MaxGPUMem); err != nil {
		return nil, err
	}

	// CPU resources next
	if a.cpu, err = AllocCPU(rqst.MaxCPU, rqst.MaxMem); err != nil {
		return nil, err
	}

	// because everything was successfully we can avoid releasing the resources we
	// have by swapping the temporary a into alloc, and clearing a
	alloc = a
	a = nil

	alloc.proj = rqst.Proj
	return alloc, nil
}

// Return any allocated resources to the sub system from which they were obtained
//
func (a *Allocated) Release() (errs []error) {

	errs = []error{}

	if a == nil {
		return []error{fmt.Errorf("unexpected nil supplied for the release of GPU resources")}
	}

	if e := ReturnGPU(a.gpu); e != nil {
		errs = append(errs, e)
	}

	if a.cpu != nil {
		a.cpu.Release()
	}

	return errs
}
