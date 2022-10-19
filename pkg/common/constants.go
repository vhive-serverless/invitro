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

	// OvercommitmentRatio Machine overcommitment ratio to provide to CPU requests in YAML specification
	OvercommitmentRatio = 10
)

type IatDistribution int

const (
	Exponential IatDistribution = iota
	Uniform     IatDistribution = iota
	Equidistant IatDistribution = iota
)

type ExperimentPhase int

const (
	WarmupPhase    ExperimentPhase = 1
	ExecutionPhase                 = 2
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
