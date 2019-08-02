package runner

// This file contains a number of tests related to measurment of CPU and memory consumption

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv"
)

func TestMetricsOutputMem(t *testing.T) {

	jbuf, err := MetricsMem()

	if err != nil {
		t.Error(err)
	}

	vMem := map[string]interface{}{}
	jsonErr := json.Unmarshal(jbuf, &vMem)

	if jsonErr != nil {
		t.Error(jsonErr)
	}

	if vMem["usedPercent"] == nil {
		t.Error("Json values missing")
	}

}

func TestMetricsOutputCPU(t *testing.T) {
	select {
	case <-time.After(time.Duration(time.Second * 2)):
	}

	jbuf, err := MetricsCPU()

	if err != nil {
		t.Error(err)
	}

	vCPU := map[string]interface{}{}
	jsonErr := json.Unmarshal(jbuf, &vCPU)

	if jsonErr != nil {
		t.Error(jsonErr)
	}
	if vCPU == nil {
		t.Error("Missing CPU Utilization")
	}
}

func TestMetricsJSON(t *testing.T) {
	select {
	case <-time.After(time.Duration(time.Second * 2)):
	}

	jbufM, _ := MetricsMem()
	jbufC, _ := MetricsCPU()

	vMem := map[string]interface{}{}
	vCPU := map[string]interface{}{}

	if errGo := json.Unmarshal(jbufM, &vMem); errGo != nil {
		t.Error(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	if errGo := json.Unmarshal(jbufC, &vCPU); errGo != nil {
		t.Error(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	vMetrics := map[string]map[string]map[string]interface{}{
		"_metrics": map[string]map[string]interface{}{
			"currentMemory": vMem,
			"currentCPU":    vCPU,
		},
	}

	jsonMetrics, errGo := json.Marshal(vMetrics)
	if errGo != nil {
		t.Error(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	vCheck := map[string]interface{}{}

	if errGo := json.Unmarshal(jsonMetrics, &vCheck); errGo != nil {
		t.Error(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}

	if _, ok := vCheck["_metrics"]; !ok {
		t.Error(kv.Wrap(fmt.Errorf("top level entity '_metrics' is missing")).With("stack", stack.Trace().TrimRuntime()))
	}
	fmt.Println(spew.Sdump(vCheck))

	select {
	case <-time.After(time.Duration(time.Second * 2)):
	}
	dump, err := MetricsAll()
	if err != nil {
		t.Error(kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()))
	}
	fmt.Println(string(dump))
}
