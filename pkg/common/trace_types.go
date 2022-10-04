package common

type FunctionConcurrencyStats struct {
	Average float64
	Count   float64
	Median  float64
	Minimum float64
	Maximum float64
	Data    []float64
}

/*type FunctionInvocationStats struct {
	Average int
	Count   int
	Median  int
	Minimum int
	Maximum int
	Data    []int
}*/

// TODO: everything above untested

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
	HashOwner string `csv:"HashOwner"`
	HashApp   string `csv:"HashApp"`

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

type Function struct {
	Name     string
	Endpoint string

	Deployed bool

	// From the profiler
	ConcurrencyStats *FunctionConcurrencyStats
	// From the trace
	InvocationStats *FunctionInvocationStats
	RuntimeStats    *FunctionRuntimeStats
	MemoryStats     *FunctionMemoryStats

	CpuRequestMilli  int
	MemoryRequestMiB int

	Specification *FunctionSpecification
}
