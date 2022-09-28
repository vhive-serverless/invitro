package common

const (
	// 1ms (min. billing unit of AWS)
	MIN_EXEC_TIME_MILLI = 1
	// 60s (avg. p96 from Wild)
	MAX_EXEC_TIME_MILLI = 60e3
)

const (
	// The stationary p-value for the ADF test that warns users if the cluster hasn't been warmed up
	// after predefined period.
	STATIONARY_P_VALUE = 0.05
	// K8s default eviction duration, after which all decisions made before should either be executed
	// or failed (and cleaned).
	PROFILING_DURATION_MINUTES = 5
	// Ten-minute warmup for unifying the starting time when the experiments consists of multiple runs.
	WARMUP_DURATION_MINUTES = 10
	// The fraction of RETURNED failures to the total invocations fired. This threshold is a patent overestimation
	// and it's here to stop the sweeping when the cluster is no longer functioning.
	OVERFLOAD_THRESHOLD = 0.3
	// The number of times allowed for the measured failure rate to surpass the `OVERFLOAD_THRESHOLD`.
	// It's here to avoid "early stopping" so that we make sure sufficient load has been imposed on the system.
	OVERFLOAD_TOLERANCE = 2
	// The compulsory timeout after which the loader will no longer await the goroutines that haven't returned,
	// and move on to the next generation round. We need it because some functions may end up in nowhere and never return.
	// By default, the wait-group will halt forever in that case.
	FORCE_TIMEOUT_MINUTE = 15
	// The portion of measurements we take in the RPS mode. The first 20% serves as a step-wise warm-up, and
	// we only take the last 80% of the measurements.
	RPS_WARMUP_FRACTION = 0.2
	// The maximum step size in the early stage of the RPS mode -- we shouldn't take too large a RPS step before reaching
	// ~100RPS in order to ensure sufficient number of measurements for lower variance (smaller the RPS, the less total data points).
	MAX_RPS_STARTUP_STEP = 5
)

type IatDistribution int

const (
	Exponential IatDistribution = iota
	Uniform     IatDistribution = iota
	Equidistant IatDistribution = iota
)

const (
	OneSecondInMicroseconds = 1_000_000.0
)
