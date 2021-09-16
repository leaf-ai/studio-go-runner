// Copyright 2021 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package cuda

import (
	"strings"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains several functions that map slots to GPUs and visa versa

// GetDevices will return a list of the possible devices that support a specified compute slot count.  The returned
// order of cards is ascending going from the smaller capacity cards to the largest and most expensive.  This function
// incorporates the AWS naming for cards when using the EC2 information functions to extract card details.
func GetDevices(slots uint) (devices []string, err kv.Error) {
	devices = []string{}
	if slots > 24 {
		return devices, kv.NewError("no cards available for this magnitude of job").With("stack", stack.Trace().TrimRuntime())
	}
	if slots <= 2 {
		smallDevices := []string{"GTX 1050", "GTX 1060", "GTX 1070", "GTX 1080", "TITAN X", "RTX 2080 Ti", "Tesla K80"}
		devices = append(devices, smallDevices...)
		devices = append(devices, "NVIDIA K80") // AWS EC2 information structure naming
	}
	if slots <= 4 {
		devices = append(devices, "Tesla P40")
	}
	if slots <= 8 {
		devices = append(devices, "Tesla P100")
	}
	if slots <= 16 {
		devices = append(devices, "Tesla V100")
		devices = append(devices, "NVIDIA V100") // AWS EC2 information structure naming
	}
	if slots <= 24 {
		devices = append(devices, "A100-SXM4-40GB")
		devices = append(devices, "NVIDIA A100") // AWS EC2 information structure naming
	}
	return devices, nil
}

// GetSlots is used to retrieved the number of compute slots that cards are capable of
func GetSlots(name string) (slots uint, err kv.Error) {
	switch {
	case strings.Contains(name, "GTX 1050"),
		strings.Contains(name, "GTX 1060"):
		slots = 2
	case strings.Contains(name, "GTX 1070"),
		strings.Contains(name, "GTX 1080"):
		slots = 2
	case strings.Contains(name, "TITAN X"):
		slots = 2
	case strings.Contains(name, "RTX 2080 Ti"):
		slots = 2
	case strings.Contains(name, "Tesla K80"),
		strings.Contains(name, "NVIDIA K80"):
		slots = 2
	case strings.Contains(name, "Tesla P40"),
		strings.Contains(name, "NVIDIA P40"):
		slots = 4
	case strings.Contains(name, "Tesla P100"),
		strings.Contains(name, "NVIDIA P100"):
		slots = 8
	case strings.Contains(name, "Tesla V100"),
		strings.Contains(name, "Tesla V100"),
		strings.Contains(name, "NVIDIA V100"):
		slots = 16
	case strings.Contains(name, "A100-SXM4-40GB"):
		slots = 24
	default:
		return 0, kv.NewError("unrecognized gpu device").With("gpu_name", name).With("stack", stack.Trace().TrimRuntime())
	}
	return slots, nil
}
