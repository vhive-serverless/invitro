package test

import (
	"testing"

	cmd "github.com/eth-easl/loader/cmd/options"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/stretchr/testify/assert"
)

func TestMaxMinAlloc(t *testing.T) {
	totalClusterCapacityMilli := 100
	scales := []int{8, 1, 9, 2, 2, 8, 0, 0}
	var functions []tc.Function
	for i := 0; i < len(scales); i++ {
		functions = append(functions, tc.Function{
			CpuRequestMilli: 1,
		})
	}

	/** Sufficient capacity. */
	newScales := cmd.MaxMinAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, scales, newScales)

	/** Max min. */
	// 9 1 8 2 2 8 -> 1 2 8 9
	// => -1
	// 8 0 7 1 1 7 (+6)
	// => -2
	// 6 0 5 0 0 5 (+8)
	// 3 1 3 2 2 3
	totalClusterCapacityMilli = 14
	newScales = cmd.MaxMinAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{3, 1, 3, 2, 2, 3, 0, 0}, newScales)

	/** Prioritising the big ones. */
	totalClusterCapacityMilli = 10
	newScales = cmd.MaxMinAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{1, 1, 5, 1, 1, 1, 0, 0}, newScales)

	/** Only prioritising the big ones. */
	totalClusterCapacityMilli = 4
	newScales = cmd.MaxMinAlloc(totalClusterCapacityMilli, scales, functions)
	assert.Equal(t, []int{0, 0, 4, 0, 0, 0, 0, 0}, newScales)
}
