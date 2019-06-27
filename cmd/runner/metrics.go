package main

//this file contains functions to be called by pythonenv.go
//these fuctions will return the values of the cpu and memory usage

import (
	"encoding/json"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

//prints out memory usage
func outputMem() (jbuf []byte, err kv.Error) {
	v, errGo := mem.VirtualMemory()

	if errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	if jbuf, errGo = json.Marshal(v); errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// returns usage percent
	return jbuf, nil

}

//returns cpu usage

func outputCPU() (jbuf []byte, errC error) {
	c, errGo := cpu.Percent(0, false)

	if errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	cpuUtil := make(map[string]float64)

	cpuUtil["cpuUtilization"] = c[0]
	if jbuf, errGo = json.Marshal(cpuUtil); errGo != nil {
		return jbuf, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	// returns usage percent
	return jbuf, nil
}
