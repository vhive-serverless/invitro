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
	"github.com/vhive-serverless/loader/pkg/common"
	"math"
	"strings"
	"testing"
)

func floatEqual(n, expected float64) bool {
	return math.Abs(n-expected) < 1e-3
}

func TestParseInvocationTrace(t *testing.T) {
	duration := 10
	invocationTrace := *parseInvocationTrace("test_data/invocations.csv", duration)

	if len(invocationTrace) != 1 {
		t.Error("Invalid invocations trace provided.")
	}

	function := invocationTrace[0]

	if function.HashOwner != "c455703077a17a9b8d0fc655d939fcc6d24d819fa9a1066b74f710c35a43cbc8" ||
		function.HashApp != "68baea05aa0c3619b6feb78c80a07e27e4e68f921d714b8125f916c3b3370bf2" ||
		function.HashFunction != "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf" ||
		function.Trigger != "queue" {

		t.Error("Unexpected data has been read.")
	}

	if len(function.Invocations) != duration {
		t.Error("Invalid invocations trace for length.")
	}

	for i := 0; i < duration; i++ {
		if function.Invocations[i] != i+1 {
			t.Error("Invalid number of invocations has been read.")
		}
	}
}

func TestParseRuntimeTrace(t *testing.T) {
	runtimeTrace := *parseRuntimeTrace("test_data/durations.csv")

	if len(runtimeTrace) != 1 {
		t.Error("Invalid runtime trace provided.")
	}

	function := runtimeTrace[0]

	if function.HashOwner != "c455703077a17a9b8d0fc655d939fcc6d24d819fa9a1066b74f710c35a43cbc8" ||
		function.HashApp != "68baea05aa0c3619b6feb78c80a07e27e4e68f921d714b8125f916c3b3370bf2" ||
		function.HashFunction != "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf" ||
		!floatEqual(function.Average, 100.0) ||
		!floatEqual(function.Count, 57523.0) ||
		!floatEqual(function.Minimum, 1.0) ||
		!floatEqual(function.Maximum, 7.0) ||
		!floatEqual(function.Percentile0, 1) ||
		!floatEqual(function.Percentile1, 2) ||
		!floatEqual(function.Percentile25, 3) ||
		!floatEqual(function.Percentile50, 4) ||
		!floatEqual(function.Percentile75, 5) ||
		!floatEqual(function.Percentile99, 6) ||
		!floatEqual(function.Percentile100, 7) {

		t.Error("Unexpected data has been read.")
	}
}

func TestParseMemoryTrace(t *testing.T) {
	memoryTrace := *parseMemoryTrace("test_data/memory.csv")

	if len(memoryTrace) != 1 {
		t.Error("Invalid memory trace provided.")
	}

	function := memoryTrace[0]

	if function.HashOwner != "c455703077a17a9b8d0fc655d939fcc6d24d819fa9a1066b74f710c35a43cbc8" ||
		function.HashApp != "68baea05aa0c3619b6feb78c80a07e27e4e68f921d714b8125f916c3b3370bf2" ||
		function.HashFunction != "c13acdc7567b225971cef2416a3a2b03c8a4d8d154df48afe75834e2f5c59ddf" ||
		!floatEqual(function.Count, 19342.0) ||
		!floatEqual(function.Average, 120.0) ||
		!floatEqual(function.Percentile1, 95) ||
		!floatEqual(function.Percentile5, 96) ||
		!floatEqual(function.Percentile25, 97) ||
		!floatEqual(function.Percentile50, 98) ||
		!floatEqual(function.Percentile75, 99) ||
		!floatEqual(function.Percentile95, 100) ||
		!floatEqual(function.Percentile99, 101) ||
		!floatEqual(function.Percentile100, 102) {

		t.Error("Unexpected data has been read.")
	}
}

func TestParserWrapper(t *testing.T) {
	parser := NewAzureParser("test_data", 10, "workloads/container/trace_func_go.yaml")
	functions := parser.Parse()

	if len(functions) != 1 {
		t.Error("Invalid function array length.")
	}
	if !strings.HasPrefix(functions[0].Name, common.FunctionNamePrefix) ||
		functions[0].InvocationStats == nil ||
		functions[0].RuntimeStats == nil ||
		functions[0].MemoryStats == nil {

		t.Error("Unexpected results.")
	}
}
