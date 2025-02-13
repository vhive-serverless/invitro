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

package common

import "container/list"

type FunctionInvocationStats struct {
	HashOwner    string
	HashApp      string
	HashFunction string
	Trigger      string

	Invocations []int
}

type FunctionRuntimeStats struct {
	HashOwner    string `csv:"HashOwner"`
	HashApp      string `csv:"HashApp"`
	HashFunction string `csv:"HashFunction"`

	Average float64 `csv:"Average"`
	Count   float64 `csv:"Count"`
	Minimum float64 `csv:"Minimum"`
	Maximum float64 `csv:"Maximum"`

	Percentile0   float64 `csv:"percentile_Average_0"`
	Percentile1   float64 `csv:"percentile_Average_1"`
	Percentile25  float64 `csv:"percentile_Average_25"`
	Percentile50  float64 `csv:"percentile_Average_50"`
	Percentile75  float64 `csv:"percentile_Average_75"`
	Percentile99  float64 `csv:"percentile_Average_99"`
	Percentile100 float64 `csv:"percentile_Average_100"`
}

type FunctionMemoryStats struct {
	HashOwner    string `csv:"HashOwner"`
	HashApp      string `csv:"HashApp"`
	HashFunction string `csv:"HashFunction"`

	Count   float64 `csv:"SampleCount"`
	Average float64 `csv:"AverageAllocatedMb"`

	Percentile1   float64 `csv:"AverageAllocatedMb_pct1"`
	Percentile5   float64 `csv:"AverageAllocatedMb_pct5"`
	Percentile25  float64 `csv:"AverageAllocatedMb_pct25"`
	Percentile50  float64 `csv:"AverageAllocatedMb_pct50"`
	Percentile75  float64 `csv:"AverageAllocatedMb_pct75"`
	Percentile95  float64 `csv:"AverageAllocatedMb_pct95"`
	Percentile99  float64 `csv:"AverageAllocatedMb_pct99"`
	Percentile100 float64 `csv:"AverageAllocatedMb_pct100"`
}

type DirigentMetadata struct {
	HashFunction        string   `json:"HashFunction"`
	Image               string   `json:"Image"`
	Port                int      `json:"Port"`
	Protocol            string   `json:"Protocol"`
	ScalingUpperBound   int      `json:"ScalingUpperBound"`
	ScalingLowerBound   int      `json:"ScalingLowerBound"`
	IterationMultiplier int      `json:"IterationMultiplier"`
	IOPercentage        int      `json:"IOPercentage"`
	EnvVars             []string `json:"EnvVars"`
	ProgramArgs         []string `json:"ProgramArgs"`
}

type Function struct {
	Name     string
	Endpoint string

	// From the static trace profiler
	InitialScale int
	// From the trace
	InvocationStats  *FunctionInvocationStats
	RuntimeStats     *FunctionRuntimeStats
	MemoryStats      *FunctionMemoryStats
	DirigentMetadata *DirigentMetadata

	ColdStartBusyLoopMs int

	CPURequestsMilli  int
	MemoryRequestsMiB int
	CPULimitsMilli    int
	YAMLPath          string
	PredeploymentPath []string
	Specification     *FunctionSpecification
}

type Node struct {
	Function *Function
	Branches []*list.List
	Depth    int
	DAG      string
}
