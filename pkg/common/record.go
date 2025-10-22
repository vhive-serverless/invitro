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

type StartType string

const (
	Hot  StartType = "hot"
	Cold StartType = "cold"
)

type MinuteInvocationRecord struct {
	Phase           int   `csv:"phase"`
	Rps             int   `csv:"rps"`
	MinuteIdx       int   `csv:"index"`
	Duration        int64 `csv:"duration"`
	NumFuncTargeted int   `csv:"num_func_target"`
	NumFuncInvoked  int   `csv:"num_func_invoked"`
	NumColdStarts   int   `csv:"num_coldstarts"`
}

type ExecutionRecordBase struct {
	Phase        int    `csv:"phase"`
	Instance     string `csv:"instance"`
	InvocationID string `csv:"invocationID"`
	StartTime    int64  `csv:"startTime"`

	// Measurements in microseconds
	RequestedDuration           uint32 `csv:"requestedDuration"`
	GRPCConnectionEstablishTime int64  `csv:"grpcConnEstablish"`
	ResponseTime                int64  `csv:"responseTime"`
	ActualDuration              uint32 `csv:"actualDuration"`

	ConnectionTimeout bool `csv:"connectionTimeout"`
	FunctionTimeout   bool `csv:"functionTimeout"`
}

type ExecutionRecordOpenWhisk struct {
	ExecutionRecordBase

	ActivationID   string    `csv:"activationID"`
	StartType      StartType `csv:"startType"`
	HttpStatusCode int       `csv:"httpStatusCode"`

	// Measurements in microseconds
	WaitTime int64 `csv:"waitTime"`
	InitTime int64 `csv:"initTime"`
}

type ExecutionRecord struct {
	ExecutionRecordBase

	// Measurements in microseconds
	ActualMemoryUsage       uint32 `csv:"actualMemoryUsage"`
	MemoryAllocationTimeout bool   `csv:"memoryAllocationTimeout"`

	AsyncResponseID     string `csv:"-"`
	TimeToSubmitMs      int64  `csv:"timeToSubmitMs"`
	UserCodeExecutionMs int64  `csv:"userCodeExecutionMs"`

	TimeToGetResponseMs int64 `csv:"timeToGetResponseMs"`
}
