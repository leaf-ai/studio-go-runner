package runner

// This file contains functions that return memory and CPU consumption

import (
	"encoding/json"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

// MetricsMem returns the current memory usage as a json serialized string
//
func MetricsMem() (jbuf []byte, err kv.Error) {

	v, errGo := mem.VirtualMemory()
	if errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if jbuf, errGo = json.Marshal(v); errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	return jbuf, nil

}

// MetricsCPU returns the cpu usage since it was last sampled.  The value is
// returned as a json serialized string
func MetricsCPU() (jbuf []byte, err kv.Error) {

	c, errGo := cpu.Percent(0, false)
	if errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	cpuUtil := map[string]float64{
		"cpuUtilization": c[0],
	}

	if jbuf, errGo = json.Marshal(cpuUtil); errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// returns a JSON serialized usage percent
	return jbuf, nil
}

// MetricsAll can be used to get a json serialized structure of the available CPU
// memory and other resource consumption statistics that are available and ready to be
// output in a human readable form
func MetricsAll() (jsonMetrics []byte, err kv.Error) {

	c, errGo := cpu.Percent(0, false)
	if errGo != nil {
		return jsonMetrics, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	v, errGo := mem.VirtualMemory()
	if errGo != nil {
		return jsonMetrics, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	type CPU struct {
		Utilization float64
	}
	type Memory struct {
		Current *mem.VirtualMemoryStat
	}

	type Metrics struct {
		CPU    CPU
		Memory Memory
	}
	type wrapper struct {
		Metrics Metrics `json:"_metrics"`
	}

	vMetrics := Metrics{
		CPU: CPU{
			Utilization: c[0],
		},
		Memory: Memory{
			Current: v,
		},
	}

	jsonMetrics, errGo = json.Marshal(wrapper{Metrics: vMetrics})
	if errGo != nil {
		return jsonMetrics, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return jsonMetrics, nil
}
