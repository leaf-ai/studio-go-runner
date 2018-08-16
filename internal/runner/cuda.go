package runner

// This file contains the data structures used by the CUDA package that are used
// for when the platform is and is not supported

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/lthibault/jitterbug"
)

type device struct {
	UUID       string        `json:"uuid"`
	Name       string        `json:"name"`
	Temp       uint          `json:"temp"`
	Powr       uint          `json:"powr"`
	MemTot     uint64        `json:"memtot"`
	MemUsed    uint64        `json:"memused"`
	MemFree    uint64        `json:"memfree"`
	EccFailure *errors.Error `json:"eccfailure"`
}

type cudaDevices struct {
	Devices []device `json:"devices"`
}

type GPUTrack struct {
	UUID       string        // The UUID designation for the GPU being managed
	Group      string        // The user grouping to which this GPU has been bound
	Slots      uint          // The number of logical slots the GPU based on its size has
	Mem        uint64        // The amount of memory the GPU posses
	FreeSlots  uint          // The number of free logical slots the GPU has available
	FreeMem    uint64        // The amount of free memory the GPU has
	EccFailure *errors.Error // Any Ecc failure related error messages, nil if no errors encounted
}

type gpuTracker struct {
	Allocs map[string]*GPUTrack
	sync.Mutex
}

var (
	// A map keyed on the nvidia device UUID containing information about cards and
	// their occupancy by the go runner.
	//
	gpuAllocs gpuTracker

	// UseGPU is used for specific types of testing to disable GPU tests when there
	// are GPU cards potentially present but they need to be disabled, this flag
	// is not used during production to change behavior in any way
	UseGPU *bool
)

func init() {
	temp := true
	UseGPU = &temp

	gpuDevices, _ := getCUDAInfo()

	visDevices := strings.Split(os.Getenv("CUDA_VISIBLE_DEVICES"), ",")

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()
	gpuAllocs.Allocs = make(map[string]*GPUTrack, len(visDevices))

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
			gpuAllocs.Allocs[gpuDevices.Devices[i].UUID] = &GPUTrack{}
		} else {
			gpuAllocs.Allocs[id] = &GPUTrack{}
		}
	}

	if len(gpuAllocs.Allocs) == 0 {
		for _, dev := range gpuDevices.Devices {
			gpuAllocs.Allocs[dev.UUID] = &GPUTrack{}
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

		track := &GPUTrack{
			UUID:       dev.UUID,
			Mem:        dev.MemFree,
			Slots:      1,
			FreeSlots:  1,
			EccFailure: dev.EccFailure,
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

// Having initialized all of the devices in the tracking map a go func
// is started that will check the devices for ECC and other errors marking
// failed GPUs
//
func MonitorGPUs(ctx context.Context, errC chan<- errors.Error) {
	t := jitterbug.New(time.Second*30, &jitterbug.Norm{Stdev: time.Second * 3})
	defer t.Stop()

	for {
		select {
		case <-t.C:
			gpuDevices, err := getCUDAInfo()
			if err != nil {
				select {
				case errC <- err:
				default:
					fmt.Println(err)
				}
			}
			// Look at allhe GPUs we have in our hardware config
			for _, dev := range gpuDevices.Devices {
				if dev.EccFailure != nil {
					gpuAllocs.Lock()
					// Check to see if the hardware GPU had a failure
					// and if it is in the tracking table and does
					// not yet have an error logged log the error
					// in the tracking table
					if gpu, isPresent := gpuAllocs.Allocs[dev.UUID]; isPresent {
						if gpu.EccFailure == nil {
							gpu.EccFailure = dev.EccFailure
							gpuAllocs.Allocs[gpu.UUID] = gpu
						}
					}
					gpuAllocs.Unlock()
				}
			}
		case <-ctx.Done():
			return
		}
	}
}

// GPUSlots gets the free and total number of GPU capacity slots within
// the machine
//
func GPUSlots() (cnt uint, freeCnt uint) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		cnt += alloc.Slots
		freeCnt += alloc.FreeSlots
	}
	return cnt, freeCnt
}

// LargestFreeGPUSlots gets the largest number of single device free GPU slots
//
func LargestFreeGPUSlots() (cnt uint) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		if alloc.FreeSlots > cnt {
			cnt = alloc.FreeSlots
		}
	}
	return cnt
}

func LargestFreeGPUMem() (freeMem uint64) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, alloc := range gpuAllocs.Allocs {
		if alloc.Slots != 0 && alloc.FreeMem > freeMem {
			freeMem = alloc.FreeMem
		}
	}
	return freeMem
}

// FindGPUs is used to locate all GPUs matching the criteria within the
// parameters supplied.  The free values specify minimums for resources.
// If the pgroup is not set then the GPUs not assigned to any group will
// be selected using the free values, and if it is specified then
// the group must match along with the minimums for the resources.  Any GPUs
// that have recorded Ecc errors will not be included
//
func FindGPUs(group string, freeSlots uint, freeMem uint64) (gpus map[string]GPUTrack) {

	gpus = map[string]GPUTrack{}

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	for _, gpu := range gpuAllocs.Allocs {
		if group == gpu.Group && gpu.EccFailure == nil &&
			freeSlots <= gpu.FreeSlots && freeMem <= gpu.FreeMem {
			gpus[gpu.UUID] = *gpu
		}
	}
	return gpus
}

type GPUAllocated struct {
	cudaDev string            // The device identifier this allocation was successful against
	group   string            // The users group that the allocation was made for
	slots   uint              // The number of GPU slots given from the allocation
	mem     uint64            // The amount of memory given to the allocation
	Env     map[string]string // Any environment variables the device allocator wants the runner to use
}

// DumpGPU is used to return to a monitoring system a JSOBN based representation of the current
// state of GPU allocations
//
func DumpGPU() (dump string) {
	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	b, err := json.Marshal(&gpuAllocs)
	if err != nil {
		return ""
	}
	return string(b)
}

// AllocGPU will attempt to find a free CUDA capable GPU and assign it to the client.  It will
// on finding a device set the appropriate values in the allocated return structure that the client
// can use to manage their resource consumption to match the permitted limits.
//
// At this time allocations cannot occur across multiple devices, only within a single device.
//
func AllocGPU(group string, maxGPU uint, maxGPUMem uint64) (alloc *GPUAllocated, err errors.Error) {

	alloc = &GPUAllocated{
		Env: map[string]string{},
	}

	if maxGPU == 0 {
		return alloc, nil
	}

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	// Look for any free slots inside the inventory that are either completely free or occupy a card already
	// that has some free slots left
	//
	matchedDevice := ""

	for _, dev := range gpuAllocs.Allocs {
		if dev.Group == "" {
			if dev.FreeSlots >= maxGPU && dev.FreeMem >= maxGPUMem && dev.EccFailure == nil {
				matchedDevice = dev.UUID
			}
			continue
		}
		// Pack the work in naively, enhancements could include looking for the best
		// fitting gaps etc
		if dev.Group == group && dev.FreeSlots >= maxGPU && dev.FreeMem >= maxGPUMem && dev.EccFailure == nil {
			matchedDevice = dev.UUID
			break
		}
	}

	if matchedDevice == "" {
		return nil, errors.New(fmt.Sprintf("no available slots where found for group %s", group)).With("stack", stack.Trace().TrimRuntime())
	}

	// Determine number of slots that could be allocated and the max requested
	//
	slots := maxGPU
	if slots > gpuAllocs.Allocs[matchedDevice].FreeSlots {
		slots = gpuAllocs.Allocs[matchedDevice].FreeSlots
	}

	if maxGPUMem == 0 {
		// If the user does not know take it all, burn it to the ground
		slots = gpuAllocs.Allocs[matchedDevice].FreeSlots
		maxGPUMem = gpuAllocs.Allocs[matchedDevice].FreeMem
	}
	gpuAllocs.Allocs[matchedDevice].Group = group
	gpuAllocs.Allocs[matchedDevice].FreeSlots -= slots
	gpuAllocs.Allocs[matchedDevice].FreeMem -= maxGPUMem

	alloc = &GPUAllocated{
		cudaDev: matchedDevice,
		group:   group,
		slots:   slots,
		mem:     maxGPUMem,
		Env:     map[string]string{"CUDA_VISIBLE_DEVICES": matchedDevice},
	}

	return alloc, nil
}

// ReturnGPU releases the GPU allocation passed in.  It will validate some of the allocation
// details but is an honors system.
//
func ReturnGPU(alloc *GPUAllocated) (err errors.Error) {

	if alloc.slots == 0 || alloc.mem == 0 {
		return nil
	}

	gpuAllocs.Lock()
	defer gpuAllocs.Unlock()

	// Make sure that the allocation is still valid
	dev, isPresent := gpuAllocs.Allocs[alloc.cudaDev]
	if !isPresent {
		return errors.New(fmt.Sprintf("cuda device %s is no longer in service", alloc.cudaDev)).With("stack", stack.Trace().TrimRuntime())
	}

	// Make sure the device was not reset and is now doing something else entirely
	if dev.Group != alloc.group {
		return errors.New(fmt.Sprintf("cuda device %s is no longer assigned to group %s, instead it is running %s", alloc.cudaDev, alloc.group, dev.Group)).With("stack", stack.Trace().TrimRuntime())
	}

	gpuAllocs.Allocs[alloc.cudaDev].FreeSlots += alloc.slots
	gpuAllocs.Allocs[alloc.cudaDev].FreeMem += alloc.mem

	// If there is no work running or left on the GPU drop it from the group constraint
	//
	if gpuAllocs.Allocs[alloc.cudaDev].FreeSlots == gpuAllocs.Allocs[alloc.cudaDev].Slots &&
		gpuAllocs.Allocs[alloc.cudaDev].FreeMem == gpuAllocs.Allocs[alloc.cudaDev].Mem {
		gpuAllocs.Allocs[alloc.cudaDev].Group = ""
	}

	return nil
}
