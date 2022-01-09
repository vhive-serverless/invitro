package trace

import (
	"fmt"
	"math"

	util "github.com/eth-easl/loader/internal"
	"github.com/montanaflynn/stats"
)

type FunctionConcurrencyStats struct {
	Average float64
	Count   float64
	Median  float64
	Minimum float64
	Maximum float64
	data    []float64
}

type FunctionInvocationStats struct {
	Average int
	Count   int
	Median  int
	Minimum int
	Maximum int
	data    []int
}
type FunctionDurationStats struct {
	average       int
	count         int
	minimum       int
	maximum       int
	percentile0   int
	percentile1   int
	percentile25  int
	percentile50  int
	percentile75  int
	percentile99  int
	percentile100 int
}

type FunctionMemoryStats struct {
	average       int
	count         int
	percentile1   int
	percentile5   int
	percentile25  int
	percentile50  int
	percentile75  int
	percentile95  int
	percentile99  int
	percentile100 int
}

type Function struct {
	name            string
	url             string
	appHash         string
	hash            string
	deployed        bool
	ConcurrencySats FunctionConcurrencyStats
	InvocationStats FunctionInvocationStats
	DurationStats   FunctionDurationStats
	MemoryStats     FunctionMemoryStats
}

type FunctionTraces struct {
	path                      string
	Functions                 []Function
	WarmupScales              []int
	InvocationsEachMinute     [][]int
	TotalInvocationsPerMinute []int
}

const (
	MAX_CONCURRENCY = 50
	MIN_CONCURRENCY = 2
)

func (f *Function) GetExpectedConcurrency() int {
	expectedRps := f.InvocationStats.Median / 60
	expectedFinishingRatePerSec := float64(f.DurationStats.percentile100) / 1000
	expectedConcurrency := float64(expectedRps) * expectedFinishingRatePerSec

	// log.Info(expectedRps, expectedFinishingRatePerSec, expectedConcurrency)

	return util.MaxOf(
		MIN_CONCURRENCY,
		util.MinOf(
			MAX_CONCURRENCY,
			int(math.Ceil(expectedConcurrency)),
		),
	)
}

func ProfileFunctionConcurrencies(function Function, duration int) FunctionConcurrencyStats {
	var concurrencies []float64
	for _, numInocations := range function.InvocationStats.data[:duration] {
		expectedRps := numInocations / 60
		expectedDepartureRatePerSec := float64(function.DurationStats.percentile100) / 1000
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

func (f *Function) SetHash(hash int) {
	f.hash = fmt.Sprintf("%015d", hash)
}

func (f *Function) SetName(name string) {
	f.name = name
}

func (f *Function) SetStatus(b bool) {
	f.deployed = b
}

func (f *Function) GetStatus() bool {
	return f.deployed
}

func (f *Function) GetName() string {
	return f.name
}

func (f *Function) GetUrl() string {
	return f.url
}

func (f *Function) SetUrl(url string) {
	f.url = url
}
