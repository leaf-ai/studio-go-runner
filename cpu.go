package runner

// This file contains functions and data structures use to handle CPU based hardware information, along
// with CPU and memory resource accounting

import (
	"github.com/shirou/gopsutil/cpu"
)

var (
	cpuInfo []cpu.InfoStat

	allocCores int
)

func init() {
	cpuInfo, _ = cpu.Info()
	allocCores = len(cpuInfo)
}
