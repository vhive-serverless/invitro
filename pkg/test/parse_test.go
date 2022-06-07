package test

import (
	"testing"

	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/stretchr/testify/assert"
)

func TestParseInvocationTrace(t *testing.T) {
	groundTruth := []int{7, 0, 5, 16, 0, 7, 0, 5, 16, 0, 7, 0, 5, 16, 0, 7}
	functionTraces := tc.ParseInvocationTrace("../../data/traces/test/inv.csv", 1440)

	assert.Equal(t, groundTruth, functionTraces.TotalInvocationsPerMinute[:16])
}
