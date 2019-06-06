package runner

//this file contains functions to be called by pythonenv.go
//these fuctions will return the values of the cpu and memory usage

import (
	"fmt"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gosutil/mem"
)

//prints out memory usage
func outputMem() {
	fmt.Println("Mem:")
}

//prints out cpu usage
func outputCPU(){
	fmt.Println("CPU:")
}
