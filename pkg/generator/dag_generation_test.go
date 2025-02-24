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

package generator

import (
	"fmt"
	"testing"

	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

var fakeConfig *config.LoaderConfiguration = &config.LoaderConfiguration{
	Platform:                     common.PlatformKnative,
	InvokeProtocol:               "grpc",
	TracePath:                    "data/traces/example",
	OutputPathPrefix:             "test",
	EnableZipkinTracing:          true,
	GRPCConnectionTimeoutSeconds: 5,
	GRPCFunctionTimeoutSeconds:   15,
	DAGMode:                      true,
	EnableDAGDataset:             false,
	Width:                        2,
	Depth:                        2,
}

var functions []*common.Function = []*common.Function{
	{
		Name: "test-function",
		InvocationStats: &common.FunctionInvocationStats{
			Invocations: []int{
				5, 5, 5, 5, 5,
				5, 5, 5, 5, 5,
				5, 5, 5, 5, 5,
				5, 5, 5, 5, 5,
			},
		},
		RuntimeStats: &common.FunctionRuntimeStats{
			Average:       50,
			Count:         100,
			Minimum:       0,
			Maximum:       100,
			Percentile0:   0,
			Percentile1:   1,
			Percentile25:  25,
			Percentile50:  50,
			Percentile75:  75,
			Percentile99:  99,
			Percentile100: 100,
		},
		MemoryStats: &common.FunctionMemoryStats{
			Average:       5000,
			Count:         100,
			Percentile1:   100,
			Percentile5:   500,
			Percentile25:  2500,
			Percentile50:  5000,
			Percentile75:  7500,
			Percentile95:  9500,
			Percentile99:  9900,
			Percentile100: 10000,
		},
		Specification: &common.FunctionSpecification{
			PerMinuteCount: []int{1},
		},
	},
}

func TestGenerateSingleDAG(t *testing.T) {
	var functionList []*common.Function = make([]*common.Function, 3)
	for i := 0; i < len(functionList); i++ {
		functionList[i] = functions[0]
	}
	dagList := GenerateDAGs(fakeConfig, functionList, true)[0]
	branch := dagList.Front().Value.(*common.Node).Branches
	if dagList.Len() != 2 && len(branch) != 1 {
		t.Error("Invalid DAG Generated")
	}
}

func TestGenerateMultipleDAGs(t *testing.T) {
	var functionList []*common.Function = make([]*common.Function, 200)
	var initialWidth int64
	for i := 0; i < len(functionList); i++ {
		functionList[i] = functions[0]
	}
	fakeConfig.Width = 10
	fakeConfig.Depth = 5
	dagList := GenerateDAGs(fakeConfig, functionList, true)
	if len(dagList) < 2 {
		t.Error("Failed to create Multiple DAGs")
	}
	for i := 0; i < len(dagList); i++ {
		initialWidth = 1
		width, depth := GetDAGShape(dagList[i], &initialWidth, 0)
		if width != fakeConfig.Width || depth != fakeConfig.Depth {
			errorMsg := fmt.Sprintf("Invalid DAG Shape: Expected Width = 10, Depth = 5. Got Width = %d, Depth = %d", width, depth)
			t.Error(errorMsg)
		}
	}
}

func TestGenerateDAGByDataset(t *testing.T) {
	var functionList []*common.Function = make([]*common.Function, 10)
	for i := 0; i < len(functionList); i++ {
		functionList[i] = functions[0]
	}
	fakeConfig.EnableDAGDataset = true
	fakeConfig.TracePath = fmt.Sprintf("../../%s", fakeConfig.TracePath)

	dagList := GenerateDAGs(fakeConfig, functionList, true)
	if len(dagList) == 0 {
		t.Error("Unable to generate DAGs by Dataset")
	}
}
