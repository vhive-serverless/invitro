package trace

import (
	"github.com/montanaflynn/stats"
)

const (
	MAX_CONCURRENCY = 50
	MIN_CONCURRENCY = 2
)

func ProfileFunctionConcurrencies(function Function, duration int) FunctionConcurrencyStats {
	var concurrencies []float64
	for _, numInvocationsPerMinute := range function.InvocationStats.data[:duration] {
		//* Compute arrival rate (unit: 1s).
		expectedRps := numInvocationsPerMinute / 60
		//* Compute processing rate (= runtime_in_milli / 1000) assuming it can be process right away upon arrival.
		expectedDepartureRatePerSec := float64(function.RuntimeStats.Percentile100) / 1000
		//* Expected concurrency == the inventory (total #jobs in the system) of Little's law.
		expectedConcurrency := float64(expectedRps) * expectedDepartureRatePerSec
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
