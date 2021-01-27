// Copyright 2018-2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

// This file contains the implementation of a message parser for requests
// arriving from studioml queues formatted using JSON.
//
// To parse and unparse this JSON data use the following ...
//
//    r, err := UnmarshalRequest(bytes)
//    bytes, err = r.Marshal()

import (
	"encoding/json"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License

	"github.com/leaf-ai/go-service/pkg/server"
)

// Config is a marshalled data structure used with studioml requests for defining the
// configuration of an environment used to run jobs
type Config struct {
	Cloud                  interface{}       `json:"cloud"`
	Database               Database          `json:"database"`
	SaveWorkspaceFrequency string            `json:"saveWorkspaceFrequency"`
	Lifetime               string            `json:"experimentLifetime"`
	Verbose                string            `json:"verbose"`
	Env                    map[string]string `json:"env"`
	Pip                    []string          `json:"pip"`
	Runner                 RunnerCustom      `json:"runner"`
}

// RunnerCustom defines a custom type of resource used by the go runner to implement a slack
// notification mechanism
//
type RunnerCustom struct {
	SlackDest string `json:"slack_destination"`
}

// Database marshalls the studioML database specification for experiment meta data
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

// Experiment marshalls the studioML experiment meta data
type Experiment struct {
	Args               []string            `json:"args"`
	Artifacts          map[string]Artifact `json:"artifacts"`
	Filename           string              `json:"filename"`
	Git                interface{}         `json:"git"`
	Info               Info                `json:"info"`
	Key                string              `json:"key"`
	Metric             interface{}         `json:"metric"`
	Project            interface{}         `json:"project"`
	Pythonenv          []string            `json:"pythonenv"`
	PythonVer          string              `json:"pythonver"`
	Resource           server.Resource     `json:"resources_needed"`
	Status             string              `json:"status"`
	TimeAdded          float64             `json:"time_added"`
	MaxDuration        string              `json:"max_duration"`
	TimeFinished       interface{}         `json:"time_finished"`
	TimeLastCheckpoint interface{}         `json:"time_last_checkpoint"`
	TimeStarted        interface{}         `json:"time_started"`
}

// Request marshalls the requests made by studioML under which all of the other
// meta data can be found
type Request struct {
	Config     Config     `json:"config"`
	Experiment Experiment `json:"experiment"`
}

// Info is a marshalled item from the studioML experiment definition that
// is ignored by the go runner and so is stubbed out
type Info struct {
}

// Artifact is a marshalled component of a StudioML experiment definition that
// is used to encapsulate files and other external data sources
// that the runner retrieve and/or upload as the experiment progresses
type Artifact struct {
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
	Hash      string `json:"hash,omitempty"`
	Local     string `json:"local,omitempty"`
	Mutable   bool   `json:"mutable"`
	Unpack    bool   `json:"unpack"`
	Qualified string `json:"qualified"`
}

// Clone is a full on duplication of the original artifact
func (a *Artifact) Clone() (b *Artifact) {
	return &Artifact{
		Bucket:    a.Bucket[:],
		Key:       a.Key[:],
		Hash:      a.Hash[:],
		Local:     a.Local[:],
		Mutable:   a.Mutable,
		Unpack:    a.Unpack,
		Qualified: a.Qualified[:],
	}
}

// UnmarshalRequest takes an encoded StudioML request and extracts it
// into go data structures used by the go runner
//
func UnmarshalRequest(data []byte) (r *Request, err kv.Error) {
	r = &Request{}
	errGo := json.Unmarshal(data, r)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return r, nil
}

// Marshal takes the go data structure used to define a StudioML experiment
// request and serializes it as json to the byte array
//
func (r *Request) Marshal() (buffer []byte, err kv.Error) {
	buffer, errGo := json.Marshal(r)
	if errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return buffer, nil
}
