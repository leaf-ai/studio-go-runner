package runner

// This file contains functions and data structures use to handle CPU based hardware information, along
// with CPU and memory resource accounting

import (
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

var (
	cpuInfo = make([]cpu.InfoStat, 0)

	allocCores int
)

func init() {

	cpuInfo, _ = cpu.InfoStat()
	allocCores = len(cpuInfo)
}
