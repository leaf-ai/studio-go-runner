package main

import (
	"encoding/json"
	"testing"
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

	var vCPU map[string]interface{}
	jsonErr := json.Unmarshal(jbuf, &vCPU)

	if jsonErr != nil {
		t.Error(jsonErr)
	}
	if vCPU == nil {
		t.Error("Missing CPU Utilization")
	} else {
		t.Logf("success, CPU: %v", vCPU["cpuUtilization"])
	}
}

func TestWrapJSON(t *testing.T) {
	jbufM, _ := outputMem()
	jbufC, _ := outputCPU()

	var vMem map[string]interface{}
	var vCPU map[string]interface{}

	json.Unmarshal(jbufM, &vMem)
	json.Unmarshal(jbufC, &vCPU)

	vJoin := map[string]map[string]interface{}{}

	vJoin["currentMemory"] = vMem
	vJoin["currentCPU"] = vCPU

	vMetrics := map[string]map[string]map[string]interface{}{}

	vMetrics["_metrics"] = vJoin

	jsonMetrics, _ := json.Marshal(vMetrics)
	t.Logf("%v", jsonMetrics)

	var vCheck map[string]interface{}

	json.Unmarshal(jsonMetrics, &vCheck)

	if _, ok := vCheck["_metrics"]; ok {
		t.Logf("%v", vCheck["_metrics"])
	}
}
