package trace

import (
	"github.com/eth-easl/loader/pkg/common"
)

func DoStaticFunctionProfiling(function *common.Function) {
	function.StaticProfilingConcurrency = profileFunctionConcurrency(function)
	// TODO: convert requests to limits
	// TODO: requests = N / 10
	// TODO: these bound should be set for non-warmup mode as well
	function.MemoryRequestMiB = int(function.MemoryStats.Percentile100)
	function.CpuRequestMilli = ConvertMemoryToCpu(function.MemoryRequestMiB)
}

func ConvertMemoryToCpu(memoryRequest int) int {
	var cpuRequest float32
	switch memoryRequest = common.MinOf(common.MAX_MEM_QUOTA_MIB, common.MaxOf(common.MIN_MEM_QUOTA_MIB, memoryRequest)); {
	// GCP conversion: https://cloud.google.com/functions/pricing
	case memoryRequest <= 128:
		cpuRequest = 0.083
	case memoryRequest <= 256:
		cpuRequest = 0.167
	case memoryRequest <= 512:
		cpuRequest = 0.333
	case memoryRequest <= 1024:
		cpuRequest = 0.583
	case memoryRequest <= 2048:
		cpuRequest = 1
	default:
		cpuRequest = 2
	}
	return int(cpuRequest * 1000)
}

func profileFunctionConcurrency(function *common.Function) float64 {
	IPM := function.InvocationStats.Invocations[0]

	// Arrival rate - unit 1 s
	rps := float64(IPM) / 60.0
	// Processing rate = runtime_in_milli / 1000, assuming it can be process right away upon arrival.
	processingRate := float64(function.RuntimeStats.Average) / 1000.0
	// Expected concurrency == the inventory (total #jobs in the system) of Little's law.
	concurrency := rps * processingRate

	return concurrency
}

/*func ProfileFunctionInvocations(invocations []int) common.FunctionInvocationStats {
	data := stats.LoadRawData(invocations)
	median, _ := stats.Median(data)
	median, _ = stats.Round(median, 0)
	max, _ := stats.Max(data)
	min, _ := stats.Min(data)
	count, _ := stats.Sum(data)
	average, _ := stats.Mean(data)

	return common.FunctionInvocationStats{
		Average: int(average),
		Count:   int(count),
		Median:  int(median),
		Minimum: int(min),
		Maximum: int(max),
		Data:    invocations,
	}
}*/
