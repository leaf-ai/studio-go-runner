// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package cpu_resource

// This file contains functions and data structures used to handle CPU based hardware information, along
// with CPU and memory resource accounting

import (
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/cpu"
	memory "github.com/shirou/gopsutil/mem"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

type cpuTracker struct {
	cpuInfo []cpu.InfoStat // CPU Information is static so cache it for later reference

	AllocCores uint   // The number of cores currently consumed and allocated
	AllocMem   uint64 // The amount of memory currently allocated

	HardMaxCores uint   // The number of cores that the hardware has provisioned
	HardMaxMem   uint64 // The amount of RAM the system has provisioned

	SoftMaxCores uint   // User specified limit on the number of cores to permit to be used in allocations
	SoftMaxMem   uint64 // User specified memory that is available for allocation

	InitErr kv.Error // Any error that might have been recorded during initialization, if set this package may produce unexpected results

	sync.Mutex
}

var (
	cpuTrack = &cpuTracker{}
)

func init() {
	cpuTrack.cpuInfo, _ = cpu.Info()

	cpuTrack.HardMaxCores = uint(len(cpuTrack.cpuInfo))
	mem, err := memory.VirtualMemory()
	if err != nil {
		cpuTrack.InitErr = kv.Wrap(err).With("stack", stack.Trace().TrimRuntime())
		return
	}
	cpuTrack.HardMaxMem = mem.Available

	cpuTrack.SoftMaxCores = cpuTrack.HardMaxCores
	cpuTrack.SoftMaxMem = cpuTrack.HardMaxMem
}

// CPUAllocated is used to track an individual allocation of CPU
// resources that will be returned at a later time
//
type CPUAllocated struct {
	Cores uint
	Mem   uint64
}

// CPUFree is used to retrieve information about the currently available CPU resources
//
func CPUFree() (cores uint, mem uint64) {
	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	return cpuTrack.SoftMaxCores - cpuTrack.AllocCores,
		cpuTrack.SoftMaxMem - cpuTrack.AllocMem
}

// SetCPULimits is used to set the soft limits for the CPU that is premitted to be allocated to
// callers
//
func SetCPULimits(maxCores uint, maxMem uint64) (err kv.Error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if cpuTrack.InitErr != nil {
		return cpuTrack.InitErr
	}

	if maxCores > cpuTrack.HardMaxCores {
		msg := fmt.Sprintf("new soft cores limit %d, violated hard limit %d", maxCores, cpuTrack.HardMaxCores)
		return kv.NewError(msg).With("stack", stack.Trace().TrimRuntime())
	}
	if maxMem > cpuTrack.HardMaxMem {
		msg := fmt.Sprintf("new soft memory limit %d, violated hard limit %d", maxMem, cpuTrack.HardMaxMem)
		return kv.NewError(msg).With("stack", stack.Trace().TrimRuntime())
	}

	if maxCores == 0 {
		cpuTrack.SoftMaxCores = cpuTrack.HardMaxCores
	} else {
		cpuTrack.SoftMaxCores = maxCores
	}

	if maxMem == 0 {
		cpuTrack.SoftMaxMem = cpuTrack.HardMaxMem
	} else {
		cpuTrack.SoftMaxMem = maxMem
	}

	return nil
}

// AllocCPU is used by callers to attempt to allocate a CPU resource from the system, CPU affinity is not implemented
// and so this is soft accounting
//
// live can be used to test the capacity is sufficient for the request without making the request itself
//
func AllocCPU(maxCores uint, maxMem uint64, live bool) (alloc *CPUAllocated, err kv.Error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if cpuTrack.InitErr != nil {
		return nil, cpuTrack.InitErr
	}

	if maxCores+cpuTrack.AllocCores > cpuTrack.SoftMaxCores {
		return nil, kv.NewError("insufficient CPU").With("cores_wanted", maxCores).With("cores_available", cpuTrack.SoftMaxCores-cpuTrack.AllocCores).With("stack", stack.Trace().TrimRuntime())
	}
	if maxMem+cpuTrack.AllocMem > cpuTrack.SoftMaxMem {
		msg := fmt.Sprintf("insufficient memory %s requested from pool of %s", humanize.Bytes(maxMem), humanize.Bytes(cpuTrack.SoftMaxMem))
		return nil, kv.NewError(msg).With("stack", stack.Trace().TrimRuntime())
	}

	if !live {
		return nil, nil
	}

	cpuTrack.AllocCores += maxCores
	cpuTrack.AllocMem += maxMem

	return &CPUAllocated{
		Cores: maxCores,
		Mem:   maxMem,
	}, nil
}

// Release is used to return a soft allocation to the system accounting
//
func (cpuAlloc *CPUAllocated) Release() {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if cpuTrack.InitErr != nil {
		return
	}

	cpuTrack.AllocCores -= cpuAlloc.Cores
	cpuTrack.AllocMem -= cpuAlloc.Mem
}
