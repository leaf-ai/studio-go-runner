package runner

import (
	"flag"
	"testing"

	"github.com/karlmutch/envflag"
)

var (
	useGPU = flag.Bool("no-gpu", false, "Used to skip test and other initialization GPU hardware code")
	useK8s = flag.Bool("use-k8s", false, "Used to enable test and other initialization for the Kubernetes cluster support")

	// TestOptions are externally visible symbols that this package is asking the unit test suite to pickup and use
	// when the testing is managed by an external entity, this allows build level variations that include or
	// exclude GPUs for example to run their tests appropriately.  It also allows the top level build logic
	// to inspect source code for executables and run their testing without knowledge of how they work.
	DuatTestOptions = [][]string{
		{""},
	}
)

func TestMain(m *testing.M) {
	// Only perform this Parsed check inside the test framework. Do not be tempted
	// to do this in the main of our production package
	//
	if !flag.Parsed() {
		envflag.Parse()
	}
	m.Run()
}

func TestStrawMan(t *testing.T) {
}
