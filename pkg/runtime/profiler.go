// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runtime // import "github.com/leaf-ai/studio-go-runner/pkg/runtime"

import (
	"context"
	"os"
	"path/filepath"
	"runtime/pprof"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the implementation of several function that handle the CPU
// profiling features offered by Go

// InitCPUProfiler is used to start a profiler for the CPU
//
func InitCPUProfiler(ctx context.Context, outputFN string) (err kv.Error) {
	if len(outputFN) == 0 {
		return kv.NewError("profiler output not specified").With("stack", stack.Trace().TrimRuntime())
	}
	output, errGo := filepath.Abs(outputFN)
	if errGo != nil {
		return kv.Wrap(errGo).With("output", outputFN).With("stack", stack.Trace().TrimRuntime())
	}
	f, errGo := os.Create(output)
	if errGo != nil {
		return kv.Wrap(errGo).With("output", outputFN).With("stack", stack.Trace().TrimRuntime())
	}
	if errGo = pprof.StartCPUProfile(f); errGo != nil {
		return kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	go cpuProfiler(ctx)

	return nil
}

func cpuProfiler(ctx context.Context) {
	defer func() {
		pprof.StopCPUProfile()
	}()
	<-ctx.Done()
}
