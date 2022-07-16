package trace

import (
	util "github.com/eth-easl/loader/pkg"
	"github.com/montanaflynn/stats"
)

const (
	MAX_CONCURRENCY = 50
	MIN_CONCURRENCY = 2

	// https://docs.aws.amazon.com/lambda/latest/dg/configuration-function-common.html#configuration-memory-console
	MAX_MEM_QUOTA_MIB = 10_240
	MIN_MEM_QUOTA_MIB = 128
)

func ProfileFunction(function Function, duration int) Function {
	function.ConcurrencySats = profileFunctionConcurrencies(function, duration)
	function.MemoryRequestMiB = function.MemoryStats.Percentile100
	function.CpuRequestMilli = convertMemoryToCpu(function.MemoryRequestMiB)
	return function
}

func convertMemoryToCpu(memoryRequest int) int {
	var cpuRequest float32
	switch memoryRequest = util.MinOf(MAX_MEM_QUOTA_MIB, util.MaxOf(MIN_MEM_QUOTA_MIB, memoryRequest)); {
	// GCP conversion: https://cloud.google.com/functions/pricing
	case memoryRequest <= 128:
		cpuRequest = 0.083
	case memoryRequest <= 512:
		cpuRequest = 0.167
	case memoryRequest <= 1024:
		cpuRequest = 0.333
	case memoryRequest <= 2024:
		cpuRequest = 1
	default:
		cpuRequest = 2
	}
	return int(cpuRequest * 1000)
}

func profileFunctionConcurrencies(function Function, duration int) FunctionConcurrencyStats {
	var concurrencies []float64
	for _, numInvocationsPerMinute := range function.InvocationStats.data[:duration] {
		//* Compute arrival rate (unit: 1s).
		expectedRps := numInvocationsPerMinute / 60
		//* Compute processing rate (= runtime_in_milli / 1000) assuming it can be process right away upon arrival.
		expectedProcessingRatePerSec := float64(function.RuntimeStats.Percentile50) / 1000
		//* Expected concurrency == the inventory (total #jobs in the system) of Little's law.
		expectedConcurrency := float64(expectedRps) * expectedProcessingRatePerSec
		concurrencies = append(concurrencies, expectedConcurrency)
	}

	data := stats.LoadRawData(concurrencies)
	median, _ := stats.Median(data)
	median, _ = stats.Round(median, 0)
	max, _ := stats.Max(data)
	min, _ := stats.Min(data)
	count, _ := stats.Sum(data)
	average, _ := stats.Mean(data)

	return FunctionConcurrencyStats{
		Average: average,
		Count:   count,
		Median:  median,
		Minimum: min,
		Maximum: max,
		data:    concurrencies,
	}
}

func ProfileFunctionInvocations(invocations []int) FunctionInvocationStats {
	data := stats.LoadRawData(invocations)
	median, _ := stats.Median(data)
	median, _ = stats.Round(median, 0)
	max, _ := stats.Max(data)
	min, _ := stats.Min(data)
	count, _ := stats.Sum(data)
	average, _ := stats.Mean(data)

	return FunctionInvocationStats{
		Average: int(average),
		Count:   int(count),
		Median:  int(median),
		Minimum: int(min),
		Maximum: int(max),
		data:    invocations,
	}
}
