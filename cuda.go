package runner

// This file contains the data structures used by the CUDA package that are used
// for when the platform is and is not supported

type device struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Temp    uint   `json:"temp"`
	Powr    uint   `json:"powr"`
	MemTot  uint64 `json:"memtot"`
	MemUsed uint64 `json:"memused"`
	MemFree uint64 `json:"memfree"`
}

type devices struct {
	Devices []device `json:"devices"`
}

var (
	gpuDevices devices
)

func init() {
	gpuDevices, _ = getCUDAInfo()
}
