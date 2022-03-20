package test

import (
	"testing"

	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/stretchr/testify/assert"
)

func TestParseInvocationTrace(t *testing.T) {
	groundTruth := []int{17703, 18058, 18058, 18058, 17853, 18060, 18057, 18058, 18058, 18057, 18058, 18058, 18057, 18059, 18057, 18057, 16571, 18058, 18059, 17244}
	functionTraces := tc.ParseInvocationTrace("../../data/traces/5/invocations.csv", 1440)

	assert.Equal(t, groundTruth, functionTraces.TotalInvocationsPerMinute[1000:1020])
}
