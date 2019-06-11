package runner

//this file contains functions to be called by pythonenv.go
//these fuctions will return the values of the cpu and memory usage

import (
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

//prints out memory usage
func outputMem() {
	v, _ := mem.VirtualMemory()
	
	// almost every return value is a struct
	fmt.Printf("Total: %v, Free:%v, UsedPercent:%f%%\n", v.Total, v.Free, v.UsedPercent)
	
	// convert to JSON. String() is also implemented
	fmt.Println(v)
}

//prints out cpu usage
func outputCPU(){
	c, _ := cpu.Info()

	fmt.Println(c)
}

