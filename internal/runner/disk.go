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
	Device     string   // The local storage device being tracked, if change this will clear our all old allocations and releases will be ignored for the old device
	AllocSpace uint64   // The amount of local storage, in the file system nominated by the user, currently allocated
	MinFree    uint64   // The amount of local storage low water mark, specified by the user, defaults to 10% of physical storage on devices
	InitErr    kv.Error // Any error that might have been recorded during initialization, if set this package may produce unexpected results

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
// space we have from a logical perspective with the headroom
// taken out
//
func GetDiskFree() (free uint64) {
	diskTrack.Lock()
	defer diskTrack.Unlock()

	fs := syscall.Statfs_t{}
	if err := syscall.Statfs(diskTrack.Device, &fs); err != nil {
		return 0
	}

	hardwareFree := fs.Bfree * uint64(fs.Bsize) // Hardware consumption check

	// Before we try to do the math with unsigned ints make sure it wont underflow
	// Make sure the hardware free space can handle the logical free space
	if hardwareFree < diskTrack.MinFree {
		return 0
	}

	highWater := (fs.Bavail * uint64(fs.Bsize)) - diskTrack.MinFree // Space available to user, allows for quotas etc, leave 10% headroom
	if highWater <= diskTrack.AllocSpace {
		return 0
	}

	return highWater - diskTrack.AllocSpace
}

// GetPathFree will use the path supplied by the caller as the device context for which
// free space information is returned, this is from a pysical free capacity
// perspective rather than what the application is allowed to allocated
// which has to adjust for minimum amount free.
//
func GetPathFree(path string) (free uint64, err kv.Error) {
	fs := syscall.Statfs_t{}
	if errGo := syscall.Statfs(path, &fs); errGo != nil {
		return 0, kv.Wrap(errGo).With("path", path).With("stack", stack.Trace().TrimRuntime())
	}

	return fs.Bfree * uint64(fs.Bsize), nil
}

// SetDiskLimits is used to set a highwater mark for a device as a minimum free quantity
//
func SetDiskLimits(device string, minFree uint64) (avail uint64, err kv.Error) {

	fs := syscall.Statfs_t{}
	if errGo := syscall.Statfs(device, &fs); err != nil {
		return 0, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	blockSize := uint64(fs.Bsize)

	softMinFree := fs.Blocks / 10 * blockSize // Minimum permitted space is 10 % of the volumes entire capacity
	if minFree > softMinFree {
		softMinFree = minFree
	}

	diskTrack.Lock()
	defer diskTrack.Unlock()

	diskTrack.AllocSpace = 0
	diskTrack.MinFree = softMinFree
	diskTrack.Device = device
	diskTrack.InitErr = nil

	return fs.Bfree*blockSize - diskTrack.MinFree, nil
}

// AllocDisk will assigned a specified amount of space from a logical bucket for the
// default disk device.  An error is returned if the available amount of disk
// is insufficient
//
func AllocDisk(maxSpace uint64, live bool) (alloc *DiskAllocated, err kv.Error) {

	avail := GetDiskFree()

	diskTrack.Lock()
	defer diskTrack.Unlock()

	if avail < maxSpace {
		return nil, kv.NewError("disk space exhausted").
			With("available", humanize.Bytes(avail), "soft_min_free", humanize.Bytes(diskTrack.MinFree),
				"allocated_already", humanize.Bytes(diskTrack.AllocSpace),
				"device", diskTrack.Device, "maxmimum_space", humanize.Bytes(maxSpace)).
			With("stack", stack.Trace().TrimRuntime())
	}

	if !live {
		return nil, nil
	}

	diskTrack.InitErr = nil
	diskTrack.AllocSpace += maxSpace

	alloc = &DiskAllocated{
		device: diskTrack.Device,
		size:   maxSpace,
	}

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
