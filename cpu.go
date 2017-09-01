package runner

// This file contains functions and data structures use to handle CPU based hardware information, along
// with CPU and memory resource accounting

import (
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

type cpuTracker struct {
	cpuInfo []cpu.InfoStat

	allocCores uint32
	allocMem   uint64

	hardMaxCores uint32 // Hardware limits on CPU consumption
	hardMaxMem   uint64 // Hardware limits of the maximum amount of memory available

	softMaxCores uint32 // User set limits on CPU consumption
	softMaxMem   uint64 // User set limit of the maximum amount of memory available

	sync.Mutex
}

var (
	cpuTrack = &cpuTracker{}
)

func init() {
	cpuTrack.cpuInfo, _ = cpu.Info()

	cpuTrack.hardMaxCores = uint32(len(cpuTrack.cpuInfo))
	mem, err := mem.VirtualMemory()
	if err != nil {
		cpuTrack.hardMaxMem = mem.Available
	}

	cpuTrack.softMaxCores = cpuTrack.hardMaxCores
	cpuTrack.softMaxMem = cpuTrack.hardMaxMem

	cpuTrack.allocCores = 0
	cpuTrack.allocMem = 0
}

type CPUAllocated struct {
	cores uint32
	mem   uint64
}

func SetCPULimits(maxCores uint32, maxMem uint64) (err error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if maxCores > cpuTrack.hardMaxCores {
		return fmt.Errorf("new soft cores limit %d, violated hard limit %d", maxCores, cpuTrack.hardMaxCores)
	}
	if maxMem > cpuTrack.hardMaxMem {
		return fmt.Errorf("new soft memory limit %d, violated hard limit %d", maxMem, cpuTrack.hardMaxMem)
	}

	cpuTrack.softMaxCores = maxCores
	cpuTrack.softMaxMem = maxMem

	return nil
}

func AllocCPU(maxCores uint32, maxMem uint64) (alloc *CPUAllocated, err error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if maxCores+cpuTrack.allocCores > cpuTrack.softMaxCores {
		return nil, fmt.Errorf("no available CPU slots found")
	}
	if maxMem+cpuTrack.allocMem > cpuTrack.softMaxMem {
		return nil, fmt.Errorf("no available memory found")
	}

	cpuTrack.allocCores += maxCores
	cpuTrack.allocMem += maxMem

	return &CPUAllocated{
		cores: maxCores,
		mem:   maxMem,
	}, nil
}

func (cpu *CPUAllocated) Release() {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	cpuTrack.allocCores -= cpu.cores
	cpuTrack.allocMem -= cpu.mem
}
