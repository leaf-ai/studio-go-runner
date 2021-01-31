// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.
package main

import (
	"fmt"

	"github.com/leaf-ai/go-service/pkg/log"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"

	"github.com/davecgh/go-spew/spew"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

var (
	logger = log.NewLogger("runner")
)

func main() {
	if _, free := cuda.GPUSlots(); free == 0 {
		if cuda.HasCUDA() {

			msg := fmt.Errorf("no available GPUs could be found using the nvidia management library")
			if cuda.CudaInitErr != nil {
				msg = *cuda.CudaInitErr
			}
			err := kv.Wrap(msg).With("stack", stack.Trace().TrimRuntime())
			logger.Fatal(fmt.Sprint(err))
		}
		gpuDevices, err := cuda.GetCUDAInfo()
		if err != nil {
			logger.Fatal(err.Error())
		}
		fmt.Println(spew.Sdump(gpuDevices))
	} else {
		logger.Fatal("No GPUs present")
	}
}
