package runner

// This file contains functions and data used to deal with local disk space allocation

import (
	"encoding/json"
	"fmt"
	"sync"
	"syscall"

	"github.com/dustin/go-humanize"
)

type diskTracker struct {
	Device string // The local storage device being tracked, if change this will clear our all old allocations and releases will be ignored for the old device

	AllocSpace uint64 // The amount of local storage, in the file system nominated by the user, currently allocated

	SoftMinFree uint64 // The amount of local storage that is available in total for allocations, specified by the user, defaults to 15% of pyshical storage on devices

	InitErr error // Any error that might have been recorded during initialization, if set this package may produce unexpected results

	sync.Mutex
}

var (
	diskTrack = &diskTracker{}
)

func initDiskResource(device string) (err error) {
	_, diskTrack.InitErr = SetDiskLimits(device, 0)
	return diskTrack.InitErr
}

// GetDiskFree is used to retrieve the amount of available disk
// space we have
//
func GetDiskFree() (free uint64) {
	diskTrack.Lock()
	defer diskTrack.Unlock()

	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(diskTrack.Device, &fs); err != nil {
		return 0
	}

	hardwareFree := uint64(float64(fs.Bavail * uint64(fs.Bsize))) // Space available to user, allows for quotas etc, leave 15% headroom

	return hardwareFree - diskTrack.SoftMinFree - diskTrack.AllocSpace
}

// DumpDisk is used by the monitoring system to dump out a JSON base representation of
// the current state of the local disk space resources allocated to the runners clients
//
func DumpDisk() (output string) {
	diskTrack.Lock()
	defer diskTrack.Unlock()

	b, err := json.Marshal(diskTrack)
	if err != nil {
		return ""
	}
	return string(b)
}

func SetDiskLimits(device string, minFree uint64) (avail uint64, err error) {

	fs := syscall.Statfs_t{}
	if err = syscall.Statfs(device, &fs); err != nil {
		return 0, err
	}

	softMinFree := uint64(float64(fs.Bavail*uint64(fs.Bsize)) * 0.15) // Space available to user, allows for quotas etc, leave 15% headroom
	if minFree != 0 && minFree < softMinFree {
		// Get the actual free space and if as
		softMinFree = minFree
	}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	if device != diskTrack.Device {
		diskTrack.AllocSpace = 0
	}
	diskTrack.SoftMinFree = softMinFree
	diskTrack.Device = device
	diskTrack.InitErr = nil

	return uint64(float64(fs.Bavail*uint64(fs.Bsize))) - diskTrack.SoftMinFree, nil
}

func AllocDisk(maxSpace uint64) (alloc *DiskAllocated, err error) {

	alloc = &DiskAllocated{}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	fs := syscall.Statfs_t{}
	if err = syscall.Statfs(diskTrack.Device, &fs); err != nil {
		return nil, err
	}

	avail := fs.Bavail * uint64(fs.Bsize)
	newAlloc := (diskTrack.AllocSpace + maxSpace)
	if avail-newAlloc <= diskTrack.SoftMinFree {
		return nil, fmt.Errorf("insufficent space %s (%s) on %s to allocate %s", humanize.Bytes(avail), humanize.Bytes(diskTrack.SoftMinFree), diskTrack.Device, humanize.Bytes(maxSpace))
	}
	diskTrack.InitErr = nil
	diskTrack.AllocSpace += maxSpace

	alloc.device = diskTrack.Device
	alloc.size = maxSpace

	return alloc, nil
}

func (alloc *DiskAllocated) Release() (err error) {

	if alloc == nil {
		return fmt.Errorf("empty allocation supplied for releasing disk storage")
	}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	if diskTrack.InitErr != nil {
		return diskTrack.InitErr
	}

	if alloc.device != diskTrack.Device {
		return fmt.Errorf("allocated space %s came from untracked local storage %s", humanize.Bytes(alloc.size), alloc.device)
	}

	diskTrack.AllocSpace -= alloc.size

	return nil
}
