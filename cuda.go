package runner

// This file contains the data structures used by the CUDA package that are used
// for when the platform is and is not supported

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
)

type device struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Temp    uint   `json:"temp"`
	Powr    uint   `json:"powr"`
	MemTot  uint64 `json:"memtot"`
	MemUsed uint64 `json:"memused"`
	MemFree uint64 `json:"memfree"`
}

type devices struct {
	Devices []device `json:"devices"`
}

type gpuTrack struct {
	UUID      string // The UUID designation for the GPU being managed
	Proj      string // The user project to which this GPU has been bound
	Slots     uint   // The number of logical slots the GPU based on its size has
	Mem       uint64 // The amount of memory the GPU posses
	FreeSlots uint   // The number of free logical slots the GPU has available
	FreeMem   uint64 // The amount of free memory the GPU has
}

type gpuTracker struct {
	Allocs map[string]*gpuTrack
	sync.Mutex
}

var (
	// A map keyed on the nvidia device UUID containing information about cards and
	// their occupancy by the go runner.
	//
	gpuAllocs gpuTracker
)

func init() {
	gpuDevices, _ := getCUDAInfo()

	visDevices := strings.Split(os.Getenv("CUDA_VISIBLE_DEVICES"), ",")

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()
	gpuAllocs.Allocs = make(map[string]*gpuTrack, len(visDevices))

	// If the visDevices were specified use then to generate existing entries inside the device map.
	// These entries will then get filled in later.
	//
	// Look to see if we have any index values in here, it really should be all UUID strings.
	// Warn if we find some, but still continue.
	warned := false
	for _, id := range visDevices {
		if len(id) == 0 {
			continue
		}
		if i, err := strconv.Atoi(id); err == nil {
			if !warned {
				warned = true
				fmt.Fprintf(os.Stderr, "CUDA_VISIBLE_DEVICES should be using UUIDs not indexes\n")
			}
			if i > len(gpuDevices.Devices) {
				fmt.Fprintf(os.Stderr, "CUDA_VISIBLE_DEVICES contained an index %d past the known population %d of GPU cards\n", i, len(gpuDevices.Devices))
			}
			gpuAllocs.Allocs[gpuDevices.Devices[i].UUID] = &gpuTrack{}
		} else {
			gpuAllocs.Allocs[id] = &gpuTrack{}
		}
	}

	if len(gpuAllocs.Allocs) == 0 {
		for _, dev := range gpuDevices.Devices {
			gpuAllocs.Allocs[dev.UUID] = &gpuTrack{}
		}
	}

	// Scan the inventory, checking matches if they were specified in the visibility env var and then fill
	// in real world data
	//
	for _, dev := range gpuDevices.Devices {
		// Dont include devices that were not specified by CUDA_VISIBLE_DEVICES
		if _, isPresent := gpuAllocs.Allocs[dev.UUID]; !isPresent {
			continue
		}

		track := &gpuTrack{
			UUID:      dev.UUID,
			Mem:       dev.MemFree,
			Slots:     1,
			FreeSlots: 1,
		}
		switch {
		case strings.Contains(dev.Name, "GTX 1050"),
			strings.Contains(dev.Name, "GTX 1060"):
			track.Slots = 1
		case strings.Contains(dev.Name, "GTX 1070"),
			strings.Contains(dev.Name, "GTX 1080"):
			track.Slots = 2
		case strings.Contains(dev.Name, "TITAN X"):
			track.Slots = 4
		}
		track.FreeSlots = track.Slots
		track.FreeMem = track.Mem
		gpuAllocs.Allocs[dev.UUID] = track
	}
}

func GetGPUCount() int {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	return len(gpuAllocs.Allocs)
}

type GpuAllocated struct {
	cudaDev string // The device identifier this allocation was successful against
	proj    string // The users project that the allocation was made for
	slots   uint   // The number of GPU slots given from the allocation
	mem     uint64 // The amount of memory given to the allocation
}

// DumpGPU is used to return to a monitoring system a JSOBN based representation of the current
// state of GPU allocations
//
func DumpGPU() (dump string) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	b, err := json.Marshal(gpuAllocs)
	if err != nil {
		return ""
	}
	return string(b)
}

func AllocGPU(proj string, maxGPU uint, maxGPUMem uint64) (alloc *GpuAllocated, err error) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	// Look for any free slots inside the inventory that are either completely free or occupy a card already
	// that has some free slots left
	//
	matchedDevice := ""

	for _, dev := range gpuAllocs.Allocs {
		if dev.Proj == "" {
			matchedDevice = dev.UUID
			continue
		}
		// Pack the work in naively, enhancements could include looking for the best
		// fitting gaps etc
		if dev.Proj == proj && dev.FreeSlots > 0 && dev.FreeMem >= maxGPUMem {
			matchedDevice = dev.UUID
			break
		}
	}

	if matchedDevice == "" {
		return nil, fmt.Errorf("no available slots where found for project %s", proj)
	}

	// Determine number of slots that could be allocated and the max requested
	//
	slots := maxGPU
	if slots > gpuAllocs.Allocs[matchedDevice].FreeSlots {
		slots = gpuAllocs.Allocs[matchedDevice].FreeSlots
	}
	gpuAllocs.Allocs[matchedDevice].Proj = proj
	gpuAllocs.Allocs[matchedDevice].FreeSlots -= slots
	gpuAllocs.Allocs[matchedDevice].FreeMem -= maxGPUMem

	alloc = &GpuAllocated{
		cudaDev: matchedDevice,
		proj:    proj,
		slots:   slots,
		mem:     maxGPUMem,
	}

	return alloc, nil
}

func ReturnGPU(alloc *GpuAllocated) (err error) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	// Make sure that the allocation is still valid
	dev, isPresent := gpuAllocs.Allocs[alloc.cudaDev]
	if !isPresent {
		return fmt.Errorf("cuda device %s is no longer in service", alloc.cudaDev)
	}

	// Make sure the device was not reset and is now doing something else entirely
	if dev.Proj != alloc.proj {
		return fmt.Errorf("cuda device %s is no longer running project %s, instead it is running %s", alloc.cudaDev, alloc.proj, dev.Proj)
	}

	gpuAllocs.Allocs[alloc.cudaDev].FreeSlots += alloc.slots
	gpuAllocs.Allocs[alloc.cudaDev].FreeMem += alloc.mem

	return nil
}
