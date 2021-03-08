// Copyright 2020 (c) Cognizant Digital Business, Evolutionary AI. All rights reserved. Issued under the Apache 2.0 License.
package runner

import (
	"sort"
	"testing"

	"github.com/go-test/deep"
	"github.com/leaf-ai/studio-go-runner/internal/cuda"
)

type gpuTestCase struct {
	alloc    *Allocated
	expected []string
}

// TestGPUEnv exercises the ability for environment settings from multiple allocated GPUs
// to be appeneded together as a single environment variable
func TestGPUEnv(t *testing.T) {
	tests := []gpuTestCase{
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: nil,
					},
				},
			},
			expected: nil,
		},
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: map[string]string{},
					},
				},
			},
			expected: nil,
		},
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "1"},
					},
				},
			},
			expected: []string{"CUDA_VISIBLE_DEVICES=1"},
		},
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "1", "CUDA_I_AM_TEAPOT": "2"},
					},
				},
			},
			expected: []string{"CUDA_VISIBLE_DEVICES=1", "CUDA_I_AM_TEAPOT=2"},
		},
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "1"},
					},
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "2"},
					},
				},
			},
			expected: []string{"CUDA_VISIBLE_DEVICES=1,2"},
		},
		gpuTestCase{
			alloc: &Allocated{
				GPU: cuda.GPUAllocations{
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "1", "CUDA_I_AM_TEAPOT": "3"},
					},
					&cuda.GPUAllocated{
						Env: map[string]string{"CUDA_VISIBLE_DEVICES": "2", "CUDA_LIFT_ME_UP_AND_POUR_ME_OUT": "4"},
					},
				},
			},
			expected: []string{"CUDA_VISIBLE_DEVICES=1,2", "CUDA_I_AM_TEAPOT=3", "CUDA_LIFT_ME_UP_AND_POUR_ME_OUT=4"},
		},
	}

	for _, aTest := range tests {
		env := gpuEnv(aTest.alloc)

		sort.Strings(aTest.expected)
		sort.Strings(env)

		if diff := deep.Equal(aTest.expected, env); diff != nil {
			t.Fatal(diff)
		}
	}
}
