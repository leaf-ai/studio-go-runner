package runner

import (
	"testing"
	"time"

	"encoding/json"
	"github.com/go-stack/stack"
	"github.com/jjeffery/kv" // MIT License
)

//this file contains tests for the go

func TestoutputMem(t *testing.T) {

	jbuf, err := outputMem()

	if err != nil {
		t.Error(err)
	}

	var vMem interface{}
	jsonErr := json.Unmarshal(jbuf, &vMem)

	if jsonErr != nill {
		t.Error(jsonErr)
	} else {
		t.Logf("success, VirtualMemory: %v", vMem)
	}

}

func TestoutputCPU(t *testing.T) {
	jbuf, err := outputCPU()

	if err != nil {
		t.Error(err)
	}

	var vMem interface{}
	jsonErr := json.Unmarshal(jbuf, &vMem)

	if jsonErr != nill {
		t.Error(jsonErr)
	} else {
		t.Logf("success, CPU: %v", vMem)
	}
}
