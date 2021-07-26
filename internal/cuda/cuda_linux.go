// +build !NO_CUDA

// Copyright 2018-2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package cuda

// This file contains the implementation and interface code for the CUDA capable devices
// that are provisioned on a system

import (
	"fmt"
	"sync"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	nvml "github.com/karlmutch/go-nvml" // MIT License

	"github.com/leaf-ai/go-service/pkg/aws_gsc" // Apache 2.0 License
)

var (
	initErr  kv.Error
	nvmlOnce sync.Once

	nvmlInit = func() {
		if errGo := nvml.NVMLInit(); errGo != nil {
			initErr = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())

			return
		}

		// If the cuda management layer started and is working then check
		// what hardware capabilities exist and print warning etc if needed as the server is started
		devs, errGo := nvml.GetAllGPUs()
		if errGo != nil {
			fmt.Println(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
			return
		}
		for _, dev := range devs {
			name, _ := dev.Name()

			uuid, errGo := dev.UUID()
			if errGo != nil {
				fmt.Println(kv.Wrap(errGo).With("name", name).With("stack", stack.Trace().TrimRuntime()))
				continue
			}

			if _, errGo = dev.MemoryInfo(); errGo != nil {
				fmt.Println(kv.Wrap(errGo).With("name", name).With("GPUID", uuid).With("stack", stack.Trace().TrimRuntime()))
				continue
			}

			if errEcc := dev.EccErrors(); errEcc != nil {
				fmt.Println(kv.Wrap(errEcc).With("name", name).With("GPUID", uuid).With("stack", stack.Trace().TrimRuntime()))
				continue
			}
		}
	}
)

// HasCUDA allows an external package to test for the presence of CUDA support
// in the go code of this package
func HasCUDA() bool {
	nvmlOnce.Do(nvmlInit)
	return true
}

func getCUDAInfo() (outDevs cudaDevices, err kv.Error) {

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
		return outDevs, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	for _, dev := range devs {

		name, _ := dev.Name()

		uuid, errGo := dev.UUID()
		if errGo != nil {
			return outDevs, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}

		temp, _ := dev.Temp()
		powr, _ := dev.PowerUsage()

		mem, errGo := dev.MemoryInfo()
		if errGo != nil {
			return outDevs, kv.Wrap(errGo).With("GPUID", uuid).With("stack", stack.Trace().TrimRuntime())
		}

		runnerDev := device{
			Name:    name,
			UUID:    uuid,
			Temp:    temp,
			Powr:    powr,
			MemTot:  mem.Total,
			MemUsed: mem.Used,
			MemFree: mem.Free,
		}
		// Dont use the ECC Error check on AWS as the NVML APIs do not appear to return the expected values
		if isAWS, _ := aws_gsc.IsAWS(); !isAWS && !CudaInTest {
			_, _, errGo := dev.EccCounts()
			if errGo != nil && errGo.Error() != "nvmlDeviceGetMemoryErrorCounter is not supported on this hardware" {
				if errEcc := dev.EccVolatileErrors(); errEcc != nil {
					err := kv.Wrap(errEcc).With("stack", stack.Trace().TrimRuntime())
					runnerDev.EccFailure = &err
				}
			}
		}
		outDevs.Devices = append(outDevs.Devices, runnerDev)
	}
	return outDevs, nil
}
