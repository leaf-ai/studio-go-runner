package runner

// This file contains the implementation of a message parser for requests
// arriving from studioml queues formatted using JSON.
//
// To parse and unparse this JSON data use the following ...
//
//    r, err := UnmarshalRequest(bytes)
//    bytes, err = r.Marshal()

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"fmt"

	"github.com/dustin/go-humanize"
)

type Resource struct {
	Cpus   uint   `json:"cpus"`
	Gpus   uint   `json:"gpus"`
	Hdd    string `json:"hdd"`
	Ram    string `json:"ram"`
	GpuMem string `json:"gpuMem"`
}

func (l *Resource) Fit(r *Resource) (didFit bool, err error) {

	lRam, err := humanize.ParseBytes(l.Ram)
	if err != nil {
		return false, fmt.Errorf("left side RAM could not be parsed")
	}

	rRam, err := humanize.ParseBytes(r.Ram)
	if err != nil {
		return false, fmt.Errorf("right side RAM could not be parsed")
	}

	lHdd, err := humanize.ParseBytes(l.Hdd)
	if err != nil {
		return false, fmt.Errorf("left side Hdd could not be parsed")
	}

	rHdd, err := humanize.ParseBytes(r.Hdd)
	if err != nil {
		return false, fmt.Errorf("right side Hdd could not be parsed")
	}

	lGpuMem, err := humanize.ParseBytes(l.GpuMem)
	if err != nil {
		return false, fmt.Errorf("left side GPUMem could not be parsed")
	}

	rGpuMem, err := humanize.ParseBytes(r.GpuMem)
	if err != nil {
		return false, fmt.Errorf("right side GPUMem could not be parsed")
	}

	return l.Cpus <= r.Cpus && l.Gpus <= r.Gpus && lHdd <= rHdd && lRam <= rRam && lGpuMem <= rGpuMem, nil
}

func (l *Resource) Clone() (r *Resource) {

	var mod bytes.Buffer
	enc := gob.NewEncoder(&mod)
	dec := gob.NewDecoder(&mod)

	if err := enc.Encode(l); err != nil {
		return nil
	}

	r = &Resource{}
	if err := dec.Decode(r); err != nil {
		return nil
	}
	return r
}

type Config struct {
	Cloud                  interface{}       `json:"cloud"`
	Resource               Resource          `json:"resource"`
	Database               Database          `json:"database"`
	SaveWorkspaceFrequency int64             `json:"saveWorkspaceFrequency"`
	Verbose                string            `json:"verbose"`
	Env                    map[string]string `json:"env"`
	Pip                    []string          `json:"pip"`
}

type Database struct {
	ApiKey            string `json:"apiKey"`
	AuthDomain        string `json:"authDomain"`
	DatabaseURL       string `json:"databaseURL"`
	MessagingSenderId int64  `json:"messagingSenderId"`
	ProjectId         string `json:"projectId"`
	StorageBucket     string `json:"storageBucket"`
	Type              string `json:"type"`
	UseEmailAuth      bool   `json:"use_email_auth"`
}

type Experiment struct {
	Args               []string            `json:"args"`
	Artifacts          map[string]Modeldir `json:"artifacts"`
	Filename           string              `json:"filename"`
	Git                interface{}         `json:"git"`
	Info               Info                `json:"info"`
	Key                string              `json:"key"`
	Metric             interface{}         `json:"metric"`
	Project            interface{}         `json:"project"`
	Pythonenv          []string            `json:"pythonenv"`
	ResourcesNeeded    ResourcesNeeded     `json:"resources_needed"`
	Status             string              `json:"status"`
	TimeAdded          float64             `json:"time_added"`
	TimeFinished       interface{}         `json:"time_finished"`
	TimeLastCheckpoint interface{}         `json:"time_last_checkpoint"`
	TimeStarted        interface{}         `json:"time_started"`
}

type Request struct {
	Config     Config     `json:"config"`
	Experiment Experiment `json:"experiment"`
}

type Info struct {
}

type Modeldir struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	Local     string `json:"local"`
	Mutable   bool   `json:"mutable"`
	Qualified string `json:"qualified"`
}

type ResourcesNeeded struct {
	Cpus int    `json:"cpus"`
	Gpus int    `json:"gpus"`
	Hdd  string `json:"hdd"`
	Ram  string `json:"ram"`
}

func UnmarshalRequest(data []byte) (r *Request, err error) {
	r = &Request{}
	err = json.Unmarshal(data, r)
	return r, err
}

func (r *Request) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
