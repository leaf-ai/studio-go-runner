package main

import (
	"fmt"
	"os"

	"github.com/SentientTechnologies/studio-go-runner/internal/runner"
	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	"github.com/prometheus/client_golang/prometheus"
)

// This file contains a set of guages and data structures for
// exporting the current set of resource assignments to prometheus

var (
	cpuFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_resource_cpu_free_slots",
			Help: "The number of CPU slots available on a host.",
		},
		[]string{"host"},
	)
	ramFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_resource_cpu_ram_free_bytes",
			Help: "The amount of CPU accessible RAM available on a host.",
		},
		[]string{"host"},
	)
	diskFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_resource_disk_free_bytes",
			Help: "The amount of free space on the working disk of a host.",
		},
		[]string{"host"},
	)
	gpuFree = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "runner_resource_gpu_free_slots",
			Help: "The the number of GPU slots available on a host.",
		},
		[]string{"host"},
	)
)

func init() {
	if errGo := prometheus.Register(cpuFree); errGo != nil {
		fmt.Fprintln(os.Stderr, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if errGo := prometheus.Register(ramFree); errGo != nil {
		fmt.Fprintln(os.Stderr, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if errGo := prometheus.Register(diskFree); errGo != nil {
		fmt.Fprintln(os.Stderr, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if errGo := prometheus.Register(gpuFree); errGo != nil {
		fmt.Fprintln(os.Stderr, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
}

func updateGauges() {
	cores, mem := runner.CPUFree()
	cpuFree.With(prometheus.Labels{"host": host}).Set(float64(cores))
	ramFree.With(prometheus.Labels{"host": host}).Set(float64(mem))

	free := runner.GetDiskFree()
	diskFree.With(prometheus.Labels{"host": host}).Set(float64(free))

	_, freeGPU := runner.GPUSlots()
	gpuFree.With(prometheus.Labels{"host": host}).Set(float64(freeGPU))
}
