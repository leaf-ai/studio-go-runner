package runner

// This file contains the implementation of a message parser for requests
// arriving from TFStudio queues formatted using JSON.
//
// To parse and unparse this JSON data use the following ...
//
//    r, err := UnmarshalRequest(bytes)
//    bytes, err = r.Marshal()

import "encoding/json"

type Request TopLevel

type Database struct {
	MessagingSenderId int64  `json:"messagingSenderId"`
	AuthDomain        string `json:"authDomain"`
	ApiKey            string `json:"apiKey"`
	DBURL             string `json:"databaseURL"`
	StorageBucket     string `json:"storageBucket"`
	ProjectId         string `json:"projectId"`
	Type              string `json:"type"`
	UseEmailAuth      bool   `json:"use_email_auth"`
}

type Cloud struct {
	Hdd  string `json:"hdd"`
	Type string `json:"type"`
	Cpus int64  `json:"cpus"`
	Gpus int64  `json:"gpus"`
	Ram  string `json:"ram"`
	Zone string `json:"zone"`
}

type Config struct {
	DB       Database `json:"database"`
	Cloud    Cloud    `json:"cloud"`
	SaveFreq int64    `json:"saveWorkspaceFrequency"`
	Verbose  string   `json:"verbose"`
}

type TopLevel struct {
	Config     Config `json:"config"`
	Experiment string `json:"experiment"`
}

func UnmarshalRequest(data []byte) (r Request, err error) {
	err = json.Unmarshal(data, &r)
	return r, err
}

func (r *Request) Marshal() ([]byte, error) {
	return json.Marshal(r)
}
