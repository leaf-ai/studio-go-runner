package runner

// This file contains functions and data structures use to handle CPU based hardware information, along
// with CPU and memory resource accounting

import (
	"fmt"
	"sync"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"

	"github.com/dustin/go-humanize"
)

type cpuTracker struct {
	cpuInfo []cpu.InfoStat

	allocCores uint
	allocMem   uint64

	hardMaxCores uint
	hardMaxMem   uint64

	softMaxCores uint
	softMaxMem   uint64

	initErr error

	sync.Mutex
}

var (
	cpuTrack = &cpuTracker{}
)

func init() {
	cpuTrack.cpuInfo, _ = cpu.Info()

	cpuTrack.hardMaxCores = uint(len(cpuTrack.cpuInfo))
	mem, err := mem.VirtualMemory()
	if err != nil {
		cpuTrack.initErr = err
		return
	}
	cpuTrack.hardMaxMem = mem.Available

	cpuTrack.softMaxCores = cpuTrack.hardMaxCores
	cpuTrack.softMaxMem = cpuTrack.hardMaxMem
}

type CPUAllocated struct {
	cores uint
	mem   uint64
}

func SetCPULimits(maxCores uint, maxMem uint64) (err error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if cpuTrack.initErr != nil {
		return cpuTrack.initErr
	}

	if maxCores > cpuTrack.hardMaxCores {
		return fmt.Errorf("new soft cores limit %d, violated hard limit %d", maxCores, cpuTrack.hardMaxCores)
	}
	if maxMem > cpuTrack.hardMaxMem {
		return fmt.Errorf("new soft memory limit %d, violated hard limit %d", maxMem, cpuTrack.hardMaxMem)
	}

	if maxCores == 0 {
		cpuTrack.softMaxCores = cpuTrack.hardMaxCores
	} else {
		cpuTrack.softMaxCores = maxCores
	}

	if maxMem == 0 {
		cpuTrack.softMaxMem = cpuTrack.hardMaxMem
	} else {
		cpuTrack.softMaxMem = maxMem
	}

	return nil
}

func AllocCPU(maxCores uint, maxMem uint64) (alloc *CPUAllocated, err error) {

	cpuTrack.Lock()
	defer cpuTrack.Unlock()

	if cpuTrack.initErr != nil {
		return nil, cpuTrack.initErr
	}

	if maxCores+cpuTrack.allocCores > cpuTrack.softMaxCores {
		return nil, fmt.Errorf("no available CPU slots found")
	}
	if maxMem+cpuTrack.allocMem > cpuTrack.softMaxMem {
		return nil, fmt.Errorf("insufficent available memory %s requested from pool of %s", humanize.Bytes(maxMem), humanize.Bytes(cpuTrack.softMaxMem))
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

	if cpuTrack.initErr != nil {
		return
	}

	cpuTrack.allocCores -= cpu.cores
	cpuTrack.allocMem -= cpu.mem
}
