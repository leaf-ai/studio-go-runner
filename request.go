package runner

// This file contains the implementation of a message parser for requests
// arriving from TFStudio queues formatted using JSON.
//
// To parse and unparse this JSON data use the following ...
//
//    r, err := UnmarshalRequest(bytes)
//    bytes, err = r.Marshal()

import "encoding/json"

type Artifacts struct {
	Modeldir  Modeldir `json:"modeldir"`
	Output    Modeldir `json:"output"`
	Tb        Modeldir `json:"tb"`
	Workspace Modeldir `json:"workspace"`
}

type Cloud struct {
	Cpus float64 `json:"cpus"`
	Gpus float64 `json:"gpus"`
	Hdd  string  `json:"hdd"`
	Ram  string  `json:"ram"`
	Type string  `json:"type"`
	Zone string  `json:"zone"`
}

type Config struct {
	Cloud                  Cloud    `json:"cloud"`
	Database               Database `json:"database"`
	SaveWorkspaceFrequency float64  `json:"saveWorkspaceFrequency"`
	Verbose                string   `json:"verbose"`
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
	Args               []string        `json:"args"`
	Artifacts          Artifacts       `json:"artifacts"`
	Filename           string          `json:"filename"`
	Git                interface{}     `json:"git"`
	Info               Info            `json:"info"`
	Key                string          `json:"key"`
	Metric             interface{}     `json:"metric"`
	Project            interface{}     `json:"project"`
	Pythonenv          []string        `json:"pythonenv"`
	ResourcesNeeded    ResourcesNeeded `json:"resources_needed"`
	Status             string          `json:"status"`
	TimeAdded          float64         `json:"time_added"`
	TimeFinished       interface{}     `json:"time_finished"`
	TimeLastCheckpoint interface{}     `json:"time_last_checkpoint"`
	TimeStarted        interface{}     `json:"time_started"`
}

type Request struct {
	Config     Config     `json:"config"`
	Experiment Experiment `json:"experiment"`
}

type Info struct {
}

type Modeldir struct {
	Key       string `json:"key"`
	Local     string `json:"local"`
	Mutable   bool   `json:"mutable"`
	Qualified string `json:"qualified"`
}

type ResourcesNeeded struct {
	Cpus float64 `json:"cpus"`
	Gpus float64 `json:"gpus"`
	Hdd  string  `json:"hdd"`
	Ram  string  `json:"ram"`
}

func UnmarshalRequest(data []byte) (r *Request, err error) {
	r = &Request{}
	err = json.Unmarshal(data, r)
	return r, err
}

func (r *Request) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
