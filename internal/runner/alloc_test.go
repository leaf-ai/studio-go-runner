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
				Tracking:   map[string]struct{}{},
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

// TestCUDAAggregateAlloc implements the minimal 2 card allocation test
//
func TestCUDAAggregateAlloc(t *testing.T) {
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
				Tracking:   map[string]struct{}{},
			},
			card2: {
				UUID:       card2,
				Slots:      1,
				Mem:        1,
				FreeSlots:  1,
				FreeMem:    1,
				EccFailure: nil,
				Tracking:   map[string]struct{}{},
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

	// maxGPU, maxGPUMem, unit of allocation
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

// TestCUDATypicalAlloc implements the multi slot 2 card allocation test
//
func TestCUDATypicalAlloc(t *testing.T) {
	card1 := xid.New().String()
	card2 := xid.New().String()

	// Test the case of two four slot cards and fit perfectedly into the requested
	// 8 slots
	testAlloc := gpuTracker{
		Allocs: map[string]*GPUTrack{
			card1: {
				UUID:       card1,
				Slots:      4,
				Mem:        2,
				FreeSlots:  4,
				FreeMem:    2,
				EccFailure: nil,
				Tracking:   map[string]struct{}{},
			},
			card2: {
				UUID:       card2,
				Slots:      4,
				Mem:        2,
				FreeSlots:  4,
				FreeMem:    2,
				EccFailure: nil,
				Tracking:   map[string]struct{}{},
			},
		},
	}

	good1Allocs, err := testAlloc.AllocGPU(8, 2, []uint{8, 4, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	// Make sure we have the expected allocation passed back
	if len(good1Allocs) != 2 {
		t.Fatal(errors.New("allocation result was unexpected").With("expected_devices", 2).With("actual_devices", len(good1Allocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	for _, anAlloc := range good1Allocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Add an 8 slot card to the two 4 slot cards and then do an 8 slot allocation to
	// ensure it finds the most efficent single card allocation
	//
	card3 := &GPUTrack{
		UUID:       xid.New().String(),
		Slots:      8,
		Mem:        2,
		FreeSlots:  8,
		FreeMem:    2,
		EccFailure: nil,
		Tracking:   map[string]struct{}{},
	}
	testAlloc.Allocs[card3.UUID] = card3

	efficentAllocs, err := testAlloc.AllocGPU(8, 2, []uint{8, 4, 2, 1})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure we have the expected allocation passed back
	if len(efficentAllocs) != 1 {
		t.Fatal(errors.New("multi-allocation result was unexpected").With("expected_devices", 1).With("actual_devices", len(efficentAllocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	for _, anAlloc := range efficentAllocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Take the 8 slot allocation and only allow 4 slot pieces and see what happens
	//
	inefficentAllocs, err := testAlloc.AllocGPU(8, 2, []uint{4, 2, 1})
	if err != nil {
		t.Fatal(err)
	}

	// Make sure we have the expected allocation passed back
	if len(inefficentAllocs) != 2 {
		t.Fatal(errors.New("multi-allocation result was unexpected").With("expected_devices", 2).With("actual_devices", len(inefficentAllocs)).With("stack", stack.Trace().TrimRuntime()))
	}

	for _, anAlloc := range inefficentAllocs {
		err = testAlloc.ReturnGPU(anAlloc)
		if err != nil {
			t.Fatal(err)
		}
	}
}
