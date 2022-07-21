package test

import (
	"testing"

	cmd "github.com/eth-easl/loader/cmd/options"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/stretchr/testify/assert"
)

func TestMaxMaxAlloc(t *testing.T) {
	totalClusterCapacityMilli := 100
	scales := []int{8, 1, 9, 2, 2, 8, 0, 0}
	var functions []tc.Function
	for i := 0; i < len(scales); i++ {
		functions = append(functions, tc.Function{
			CpuRequestMilli: 1,
		})
	}

	/** Sufficient capacity. */
	newScales := cmd.MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, scales, newScales)

	/** Max max. */
	totalClusterCapacityMilli = 27
	newScales = cmd.MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{8, 0, 9, 1, 1, 8, 0, 0}, newScales)

	totalClusterCapacityMilli = 21
	newScales = cmd.MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{6, 0, 9, 0, 0, 6, 0, 0}, newScales)

	totalClusterCapacityMilli = 4
	newScales = cmd.MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{0, 0, 4, 0, 0, 0, 0, 0}, newScales)
}
