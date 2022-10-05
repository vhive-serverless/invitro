package trace

import (
	"github.com/eth-easl/loader/pkg/common"
	"testing"
)

func TestStaticTraceProfiling(t *testing.T) {
	tests := []struct {
		testName             string
		IPM                  int
		memoryMaxPercentile  float64
		expectedInitialScale int
		expectedCPULimits    int
	}{
		{
			testName:             "concurrency_30ipm",
			IPM:                  30,
			memoryMaxPercentile:  256,
			expectedInitialScale: 1,
			expectedCPULimits:    167,
		},
		{
			testName:             "concurrency_45ipm",
			IPM:                  45,
			memoryMaxPercentile:  512,
			expectedInitialScale: 2,
			expectedCPULimits:    333,
		},
		{
			testName:             "concurrency_60ipm",
			IPM:                  60,
			memoryMaxPercentile:  1024,
			expectedInitialScale: 2,
			expectedCPULimits:    583,
		},
		{
			testName:             "concurrency_120ipm",
			IPM:                  120,
			memoryMaxPercentile:  2048,
			expectedInitialScale: 4,
			expectedCPULimits:    1000,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			f := &common.Function{
				InvocationStats: &common.FunctionInvocationStats{
					Invocations: []int{test.IPM},
				},
				RuntimeStats: &common.FunctionRuntimeStats{
					Average: 2000.0,
				},
				MemoryStats: &common.FunctionMemoryStats{
					Percentile100: test.memoryMaxPercentile,
				},
			}

			DoStaticTraceProfiling([]*common.Function{f})

			if f.InitialScale != test.expectedInitialScale ||
				f.CPULimitsMilli != test.expectedCPULimits ||
				f.CPURequestsMilli != f.CPULimitsMilli/10 {

				t.Error("Wrong static trace profile.")
			}
		})
	}
}
