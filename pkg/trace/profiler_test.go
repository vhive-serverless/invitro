/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package trace

import (
	"testing"

	"github.com/vhive-serverless/loader/pkg/common"
)

func TestStaticTraceProfiling(t *testing.T) {
	tests := []struct {
		testName             string
		CPULimit             string
		IPM                  int
		memoryMaxPercentile  float64
		expectedInitialScale int
		expectedCPULimits    int
	}{
		{
			testName:             "concurrency_30ipm",
			CPULimit:             "GCP",
			IPM:                  30,
			memoryMaxPercentile:  256,
			expectedInitialScale: 1,
			expectedCPULimits:    167,
		},
		{
			testName:             "concurrency_45ipm",
			CPULimit:             "GCP",
			IPM:                  45,
			memoryMaxPercentile:  512,
			expectedInitialScale: 2,
			expectedCPULimits:    333,
		},
		{
			testName:             "concurrency_60ipm",
			CPULimit:             "GCP",
			IPM:                  60,
			memoryMaxPercentile:  1024,
			expectedInitialScale: 2,
			expectedCPULimits:    583,
		},
		{
			testName:             "concurrency_120ipm",
			CPULimit:             "GCP",
			IPM:                  120,
			memoryMaxPercentile:  2048,
			expectedInitialScale: 4,
			expectedCPULimits:    1000,
		},
		{
			testName:             "concurrency_120ipm_1vCPU",
			CPULimit:             "1vCPU",
			IPM:                  30,
			memoryMaxPercentile:  256,
			expectedInitialScale: 1,
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
			ApplyResourceLimits([]*common.Function{f}, test.CPULimit)

			if f.InitialScale != test.expectedInitialScale ||
				f.CPULimitsMilli != test.expectedCPULimits ||
				f.CPURequestsMilli != f.CPULimitsMilli/common.OvercommitmentRatio ||
				f.MemoryRequestsMiB != int(test.memoryMaxPercentile)/common.OvercommitmentRatio {

				t.Error("Wrong static trace profile.")
			}
		})
	}
}
