// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.

package runner

import (
	"encoding/json"

	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

// This file contains the implementation of an envelop message that will be used to
// add the chrome for holding StudioML requests and their respective signature attachment
// and encryption wrappers.

type OpenExperiment struct {
	Status    string `json:"status"`
	PythonVer string `json:"pthonver"`
}

// Message contains any clear text fields and either an an encrypted payload or clear text
// payloads as a Request.
type Message struct {
	Experiment         OpenExperiment `json:"experiment"`
	TimeAdded          float64        `json:"time_added"`
	ExperimentLifetime string         `json:"experiment_lifetime"`
	Resource           Resource       `json:"resources_needed"`
	Payload            string         `json:"payload"`
	Fingerprint        string         `json:"fingerprint"`
	Signature          string         `json:"signature"`
}

// Request marshals the requests made by studioML under which all of the other
// meta data can be found
type Envelope struct {
	Message Message `json:"message"`
}

// IsEnvelop is used to test if a JSON payload is indeed present
func IsEnvelope(msg []byte) (isEnvelope bool, err kv.Error) {
	fields := map[string]interface{}{}
	if errGo := json.Unmarshal(msg, &fields); errGo != nil {
		return false, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	// Examine the fields and see that we have a message
	if _, isPresent := fields["message"]; !isPresent {
		return false, kv.NewError("'message' missing").With("stack", stack.Trace().TrimRuntime())
	}
	message, isOK := fields["message"].(map[string]interface{})
	if !isOK {
		return false, kv.NewError("'message.payload' invalid").With("stack", stack.Trace().TrimRuntime())
	}
	if _, isPresent := message["payload"]; !isPresent {
		return false, kv.NewError("'message.payload' missing").With("stack", stack.Trace().TrimRuntime())

	}
	return true, nil
}

// UnmarshalRequest takes an encoded StudioML envelope and extracts it
// into go data structures used by the go runner.
//
func UnmarshalEnvelope(data []byte) (e *Envelope, err kv.Error) {
	e = &Envelope{}
	if errGo := json.Unmarshal(data, e); errGo != nil {
		return nil, kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return e, nil
}

// Marshal takes the go data structure used to define a StudioML experiment envelope
// and serializes it as json to the byte array
//
func (e *Envelope) Marshal() ([]byte, error) {
	return json.Marshal(e)
}
