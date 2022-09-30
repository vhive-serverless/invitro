package common

type FunctionConcurrencyStats struct {
	Average float64
	Count   float64
	Median  float64
	Minimum float64
	Maximum float64
	Data    []float64
}

type FunctionInvocationStats struct {
	Average int
	Count   int
	Median  int
	Minimum int
	Maximum int
	Data    []int
}

type FunctionRuntimeStats struct {
	Average       int
	Count         int
	Minimum       int
	Maximum       int
	Percentile0   int
	Percentile1   int
	Percentile25  int
	Percentile50  int
	Percentile75  int
	Percentile99  int
	Percentile100 int
}

type FunctionMemoryStats struct {
	Average       int
	Count         int
	Percentile1   int
	Percentile5   int
	Percentile25  int
	Percentile50  int
	Percentile75  int
	Percentile95  int
	Percentile99  int
	Percentile100 int
}

type Function struct {
	Name     string
	Endpoint string

	HashOwner    string
	HashApp      string
	HashFunction string

	Deployed bool

	NumInvocationsPerMinute []int

	ConcurrencyStats FunctionConcurrencyStats
	InvocationStats  FunctionInvocationStats
	RuntimeStats     FunctionRuntimeStats
	MemoryStats      FunctionMemoryStats

	CpuRequestMilli  int
	MemoryRequestMiB int

	Specification *FunctionSpecification
}

type FunctionTraces struct {
	Path                      string
	Functions                 []*Function
	WarmupScales              []int
	InvocationsEachMinute     [][]int
	TotalInvocationsPerMinute []int
}
