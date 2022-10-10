package common

const (
	FunctionNamePrefix      = "trace-func"
	OneSecondInMicroseconds = 1_000_000.0
)

const (
	// 1ms (min. billing unit of AWS)
	MIN_EXEC_TIME_MILLI = 1

	// 60s (avg. p96 from Wild)
	MAX_EXEC_TIME_MILLI = 60e3
)

const (
	DEFAULT_WARMUP_DURATION_MINUTES = 10

	// https://docs.aws.amazon.com/lambda/latest/dg/configuration-function-common.html#configuration-memory-console
	MAX_MEM_QUOTA_MIB = 10_240
	MIN_MEM_QUOTA_MIB = 128

	// Machine overcommitment ratio to provide to CPU requests in YAML specification
	OVERCOMMITMENT_RATIO = 10
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
