// +build !NO_CUDA

package runner

// This file contains the implementation and interface code for the CUDA capable devices
// that are provisioned on a system

import (
	"sync"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	nvml "github.com/karlmutch/go-nvml" // MIT License
)

var (
	initErr  errors.Error
	nvmlOnce sync.Once

	nvmlInit = func() {
		if errGo := nvml.NVMLInit(); errGo != nil {
			initErr = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
	}
)

func HasCUDA() bool {
	nvmlOnce.Do(nvmlInit)
	return true
}

func getCUDAInfo() (outDevs cudaDevices, err errors.Error) {

	nvmlOnce.Do(nvmlInit)

	outDevs = cudaDevices{
		Devices: []device{},
	}

	// Dont let the GetAllGPUs log a fatal error catch it first
	if initErr != nil {
		return outDevs, initErr
	}

	devs, errGo := nvml.GetAllGPUs()
	if errGo != nil {
		return outDevs, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	for _, dev := range devs {

		name, _ := dev.Name()

		uuid, errGo := dev.UUID()
		if errGo != nil {
			return outDevs, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		temp, _ := dev.Temp()
		powr, _ := dev.PowerUsage()

		mem, errGo := dev.MemoryInfo()
		if errGo != nil {
			return outDevs, errors.Wrap(errGo).With("GPUID", uuid).With("stack", stack.Trace().TrimRuntime())
		}

		errEcc := dev.EccErrors()

		runnerDev := device{
			Name:    name,
			UUID:    uuid,
			Temp:    temp,
			Powr:    powr,
			MemTot:  mem.Total,
			MemUsed: mem.Used,
			MemFree: mem.Free,
		}
		if errEcc != nil {
			err := errors.Wrap(errEcc).With("stack", stack.Trace().TrimRuntime())
			runnerDev.EccFailure = &err
		}
		outDevs.Devices = append(outDevs.Devices, runnerDev)
	}
	return outDevs, nil
}
