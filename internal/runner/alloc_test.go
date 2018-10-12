package runner

import (
	"testing"

	"github.com/go-stack/stack"
	"github.com/karlmutch/errors"
	"github.com/rs/xid"
)

// This file contains the implementations of tests
// related to resource allocation logic

// TestCUDATrivialAlloc implements the barest minimum success and failure cases with
// a single resource
//
func TestCUDATrivialAlloc(t *testing.T) {
	id := xid.New().String()
	testAlloc := gpuTracker{
		Allocs: map[string]*GPUTrack{
			id: {
				UUID:       id,
				Slots:      1,
				Mem:        1,
				FreeSlots:  1,
				FreeMem:    1,
				EccFailure: nil,
			},
		},
	}

	goodAllocs, err := testAlloc.AllocGPU(1, 1, []uint{1})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected allocation passed back
	if len(goodAllocs) != 1 {
		t.Fatal(errors.New("allocation result was unexpected").With("expected_devices", 1).With("actual_devices", len(goodAllocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	// Try to allocate a new GPU and make sure it fails
	badAllocs, err := testAlloc.AllocGPU(1, 1, []uint{1})
	if len(badAllocs) != 0 {
		t.Fatal(errors.New("allocation result should be empty").With("expected_devices", 0).With("actual_devices", len(badAllocs)).With("stack", stack.Trace().TrimRuntime()))
	}
	if err == nil {
		t.Fatal(errors.New("allocation result should have failed").With("stack", stack.Trace().TrimRuntime()))
	}
}

// TestCUDATrvialAlloc implements the barest minimum success and failure cases with
// a single resource
//
func TestCUDATrvialAlloc(t *testing.T) {
	card1 := xid.New().String()
	card2 := xid.New().String()

	testAlloc := gpuTracker{
		Allocs: map[string]*GPUTrack{
			card1: {
				UUID:       card1,
				Slots:      1,
				Mem:        1,
				FreeSlots:  1,
				FreeMem:    1,
				EccFailure: nil,
			},
			card2: {
				UUID:       card2,
				Slots:      1,
				Mem:        1,
				FreeSlots:  1,
				FreeMem:    1,
				EccFailure: nil,
			},
		},
	}

	good1Allocs, err := testAlloc.AllocGPU(1, 1, []uint{1})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected allocation passed back
	if len(good1Allocs) != 1 {
		t.Fatal(errors.New("allocation result was unexpected").With("expected_devices", 1).With("actual_devices", len(good1Allocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	good2Allocs, err := testAlloc.AllocGPU(1, 1, []uint{1})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected allocation passed back
	if len(good2Allocs) != 1 {
		t.Fatal(errors.New("allocation result was unexpected").With("expected_devices", 1).With("actual_devices", len(good2Allocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	for _, anAlloc := range good1Allocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, anAlloc := range good2Allocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}

	goodAllAllocs, err := testAlloc.AllocGPU(2, 1, []uint{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected allocation passed back
	if len(goodAllAllocs) != 2 {
		t.Fatal(errors.New("allocation result was unexpected").With("expected_devices", 2).With("actual_devices", len(goodAllAllocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	for _, anAlloc := range goodAllAllocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}
	// Now try an alloc that has already been released to make sure we get an error
	for _, anAlloc := range goodAllAllocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err == nil {
			t.Fatal(errors.New("double release did not fail").With("stack", stack.Trace().TrimRuntime()))
		}
	}
}
