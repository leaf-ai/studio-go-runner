// +build !NO_CUDA

package runner

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"

	nvml "github.com/karlmutch/go-nvml"
)

var (
	formatIssue = errors.New("unexpected format, lines should be in the format x=y")
)

// This file contains an integration test implementation that submits a studio runner
// task across an SQS queue and then validates is has completed successfully by
// the go runner this test is running within

func readIni(fn string) (items map[string]string, err errors.Error) {

	items = map[string]string{}

	fh, errGo := os.Open(fn)
	if errGo != nil {
		return items, errors.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}
	defer fh.Close()

	scanner := bufio.NewScanner(fh)
	for scanner.Scan() {
		aLine := scanner.Text()
		kv := strings.SplitN(aLine, "=", 2)
		if len(kv) != 2 {
			return items, formatIssue.With("line", aLine).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
		}
		items[kv[0]] = kv[1]
	}
	if errGo := scanner.Err(); errGo != nil {
		return items, errors.Wrap(errGo).With("filename", fn).With("stack", stack.Trace().TrimRuntime())
	}

	return items, nil
}

// TestCUDAActive checks that at least one GPU is available before any other GPU tests are used
//
func TestCUDAActive(t *testing.T) {
	logger := NewLogger("cuda_active_test")
	defer logger.Warn("completed")

	if !*UseGPU {
		logger.Warn("TestCUDA not run")
		t.Skip("no GPUs present for testing")
	}

	devs, errGo := nvml.GetAllGPUs()
	if errGo != nil {
		t.Fatal(errGo)
	}
	if len(devs) < 1 {
		t.Fatal("no CUDA capable devices found during the CUDA testing")
	}

	annotations, err := readIni(k8sAnnotations)
	if err != nil {
		logger.Warn("test appears to be running without k8s specifications")

		if *useK8s {
			t.Fatal("Kubernetes cluster present for testing, however the downward API files are missing " + err.Error())
		}
		return
	}

	if gpus, isPresent := annotations["gpus"]; isPresent {
		gpus = strings.Trim(gpus, "\"'")
		expected, errGo := strconv.Atoi(gpus)
		if errGo != nil {
			t.Fatal(errGo.Error())

		}
		if len(devs) != expected {
			t.Fatal(fmt.Sprintln("expected ", expected, " gpus got ", len(devs)))
		}
	}
}
