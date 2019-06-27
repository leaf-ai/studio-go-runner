package runner

import (
	"testing"

	"encoding/json"
)

//this file contains tests for the go

func TestOutputMem(t *testing.T) {

	jbuf, err := outputMem()

	if err != nil {
		t.Error(err)
	}

	var vMem map[string]interface{}
	jsonErr := json.Unmarshal(jbuf, &vMem)

	if jsonErr != nil {
		t.Error(jsonErr)
	}

	if vMem["usedPercent"] == nil {
		t.Error("Json values missing")
	} else {
		t.Logf("Success, Memory Used Percentage: %v", vMem["usedPercent"])
	}

}

func TestOutputCPU(t *testing.T) {
	jbuf, err := outputCPU()

	if err != nil {
		t.Error(err)
	}

	var vCPU interface{}
	jsonErr := json.Unmarshal(jbuf, &vCPU)

	if jsonErr != nil {
		t.Error(jsonErr)
	}
	if vCPU == nil {
		t.Error("Missing CPU Utilization")
	} else {
		t.Logf("success, CPU: %v", vMem)
	}
}
