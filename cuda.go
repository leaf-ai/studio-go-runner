package runner

// This file contains the implementation and interface code for the CUDA capable devices
// that are provisioned on a system
import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/karlmutch/cu"
	nvml "github.com/karlmutch/go-nvml"
)

func init() {
	err := nvml.NVMLInit()
	if err != nil {
		log.Fatal("could not initialize nvml due to ", err.Error())
	}
}

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

func getCUDAInfo() (outDevs devices, err error) {

	devs, err := nvml.GetAllGPUs()
	if err != nil {
		return nil, err
	}

	outDevs = devices{Devices: make([]device, 0, len(devs))}

	for _, dev := range devs {

		name, _ := dev.Name()
		uuid, _ := dev.UUID()
		temp, _ := dev.Temp()
		powr, _ := dev.PowerUsage()

		mem, err := dev.MemoryInfo()
		if err != nil {
			return nil, err
		}

		outDevs.Devices = append(outDevs.Devices, device{
			Name:    name,
			UUID:    uuid,
			Temp:    temp,
			Powr:    powr,
			MemTot:  mem.Total,
			MemUsed: mem.Used,
			MemFree: mem.Free,
		})
	}
	return outDevs, nil
}
