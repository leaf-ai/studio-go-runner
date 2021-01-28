// Copyright 2020-2021 (c) The Go Service Components authors. All rights reserved. Issued under the Apache 2.0 License.

package server // import "github.com/leaf-ai/go-service/pkg/server"

import (
	"os"
	"runtime"

	"github.com/dustin/go-humanize"

	"github.com/minio/minio/pkg/disk"
	memory "github.com/shirou/gopsutil/mem"
)

// Resources is a receiver for resource related methods used to describe machine level capabilities
//
type Resources struct{}

// FetchMachineResources extracts the current system state in terms of memory etc
// and coverts this into the resource specification used to pass machine characteristics
// around.
//
func (*Resources) FetchMachineResources() (rsc *Resource) {

	rsc = &Resource{
		Cpus:   uint(runtime.NumCPU()),
		Gpus:   0,
		GpuMem: "0",
	}

	v, _ := memory.VirtualMemory()
	rsc.Ram = humanize.Bytes(v.Free)

	if dir, errGo := os.Getwd(); errGo != nil {
		if di, errGo := disk.GetInfo(dir); errGo != nil {
			rsc.Hdd = humanize.Bytes(di.Total - di.Free)
		}
	}

	return rsc
}
