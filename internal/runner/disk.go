// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains functions and data used to deal with local disk space allocation

import (
	"sync"
	"syscall"

	"github.com/dustin/go-humanize"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

type diskTracker struct {
	Device string // The local storage device being tracked, if change this will clear our all old allocations and releases will be ignored for the old device

	AllocSpace uint64 // The amount of local storage, in the file system nominated by the user, currently allocated

	SoftMinFree uint64 // The amount of local storage that is available in total for allocations, specified by the user, defaults to 15% of pyshical storage on devices

	InitErr kv.Error // Any error that might have been recorded during initialization, if set this package may produce unexpected results

	sync.Mutex
}

var (
	diskTrack = &diskTracker{}
)

func initDiskResource(device string) (err kv.Error) {
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

// GetPathFree will use the path supplied by the caller as the device context for which
// free space information is returned
//
func GetPathFree(path string) (free uint64, err kv.Error) {
	fs := syscall.Statfs_t{}
	if errGo := syscall.Statfs(path, &fs); errGo != nil {
		return 0, kv.Wrap(errGo).With("path", path).With("stack", stack.Trace().TrimRuntime())
	}

	return uint64(float64(fs.Bavail * uint64(fs.Bsize))), nil
}

// SetDiskLimits is used to set a highwater mark for a device as a minimum free quantity
//
func SetDiskLimits(device string, minFree uint64) (avail uint64, err kv.Error) {

	fs := syscall.Statfs_t{}
	if errGo := syscall.Statfs(device, &fs); err != nil {
		return 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
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

// AllocDisk will assigned a specified amount of space from a logical bucket for the
// default disk device.  An error is returned if the available amount fo disk
// is insufficient
//
func AllocDisk(maxSpace uint64) (alloc *DiskAllocated, err kv.Error) {

	alloc = &DiskAllocated{}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	fs := syscall.Statfs_t{}
	if errGo := syscall.Statfs(diskTrack.Device, &fs); err != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	avail := fs.Bavail * uint64(fs.Bsize)
	newAlloc := (diskTrack.AllocSpace + maxSpace)
	if avail-newAlloc <= diskTrack.SoftMinFree {
		return nil, kv.NewError("insufficient disk space").
			With("available", humanize.Bytes(avail), "soft_min_free", humanize.Bytes(diskTrack.SoftMinFree),
				"device", diskTrack.Device, "maxmimum_space", humanize.Bytes(maxSpace)).
			With("stack", stack.Trace().TrimRuntime())
	}
	diskTrack.InitErr = nil
	diskTrack.AllocSpace += maxSpace

	alloc.device = diskTrack.Device
	alloc.size = maxSpace

	return alloc, nil
}

// Release will return assigned disk space back to the free pool of disk space
// for a default disk device.  If the allocation is not recognized then an
// error is returned
//
func (alloc *DiskAllocated) Release() (err kv.Error) {

	if alloc == nil {
		return kv.NewError("empty allocation supplied for releasing disk storage").With("stack", stack.Trace().TrimRuntime())
	}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	if diskTrack.InitErr != nil {
		return diskTrack.InitErr
	}

	if alloc.device != diskTrack.Device {
		return kv.NewError("allocated space came from untracked local storage").
			With("allocated_size", humanize.Bytes(alloc.size), "device", alloc.device).With("stack", stack.Trace().TrimRuntime())
	}

	diskTrack.AllocSpace -= alloc.size

	return nil
}
