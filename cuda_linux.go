// +build !NO_CUDA

package runner

// This file contains the implementation and interface code for the CUDA capable devices
// that are provisioned on a system

import (
	"log"

	nvml "github.com/karlmutch/go-nvml"
)

var (
	initErr = nvml.NVMLInit()
)

func init() {
	if initErr != nil {
		log.Fatal("could not initialize nvml due to ", initErr.Error())
	}
}

func HasCUDA() bool {
	return true
}

func getCUDAInfo() (outDevs cudaDevices, err error) {

	devs, err := nvml.GetAllGPUs()
	outDevs = cudaDevices{Devices: make([]device, 0, len(devs))}
	if err != nil {
		return outDevs, err
	}

	for _, dev := range devs {

		name, _ := dev.Name()
		uuid, _ := dev.UUID()
		temp, _ := dev.Temp()
		powr, _ := dev.PowerUsage()

		mem, err := dev.MemoryInfo()
		if err != nil {
			return outDevs, err
		}

		outDevs.Devices = append(outDevs.Devices, device{
			Name:    name,
			UUID:    uuid,
			Temp:    temp,
			Powr:    powr,
			MemTot:  mem.Total,
			MemUsed: mem.Used,
			MemFree: mem.Free,
		})
	}
	return outDevs, nil
}
