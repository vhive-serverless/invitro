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

const (
	FunctionNamePrefix      = "trace-func"
	OneSecondInMicroseconds = 1_000_000.0
)

const (
	// MinExecTimeMilli 1ms (min. billing unit of AWS)
	MinExecTimeMilli = 1

	// MaxExecTimeMilli 60s (avg. p96 from Wild)
	MaxExecTimeMilli = 60e3
)

const (
	// MaxMemQuotaMib Number taken from AWS Lambda settings
	// https://docs.aws.amazon.com/lambda/latest/dg/configuration-function-common.html#configuration-memory-console
	MaxMemQuotaMib = 10_240
	MinMemQuotaMib = 1

	// OvercommitmentRatio Machine overcommitment ratio to provide to CPU requests in YAML specification.
	// Value taken from the Firecracker NSDI'20 paper.
	OvercommitmentRatio = 10
)

type IatDistribution int

const (
	Exponential IatDistribution = iota
	Uniform
	Equidistant
)

type TraceGranularity int

const (
	MinuteGranularity TraceGranularity = iota
	SecondGranularity
)

type ExperimentPhase int

const (
	WarmupPhase    ExperimentPhase = 1
	ExecutionPhase ExperimentPhase = 2
)

const (
	// RequestedVsIssuedWarnThreshold Print warning on stdout if the relative difference between requested
	// and issued number of invocations is higher than this threshold
	RequestedVsIssuedWarnThreshold = 0.1
	// RequestedVsIssuedTerminateThreshold Terminate experiment if the relative difference between
	// requested and issued number of invocations is higher than this threshold
	RequestedVsIssuedTerminateThreshold = 0.2

	// FailedWarnThreshold Print warning on stdout if the percentage of failed invocations (e.g., connection timeouts,
	// function timeouts) is greater than this threshold
	FailedWarnThreshold = 0.3
	// FailedTerminateThreshold Terminate experiment if the percentage of failed invocations (e.g., connection timeouts,
	// function timeouts) is greater than this threshold
	FailedTerminateThreshold = 0.5
)

type RuntimeAssertType int

const (
	RequestedVsIssued RuntimeAssertType = 0
	IssuedVsFailed    RuntimeAssertType = 1
)

const (
	AwsRegion                  = "us-east-1"
	AwsTraceFuncRepositoryName = "invitro_trace_function_aws"
)

// CPULimits
const (
	CPULimit1vCPU string = "1vCPU"
	CPULimitGCP   string = "GCP"
)

var ValidCPULimits = []string{CPULimit1vCPU, CPULimitGCP}

// platform
const (
	PlatformKnative   string = "knative"
	PlatformDirigent  string = "dirigent"
	PlatformOpenWhisk string = "openwhisk"
	PlatformAWSLambda string = "awslambda"
)

// dirigent backend
const (
	BackendDandelion string = "dandelion"
)
