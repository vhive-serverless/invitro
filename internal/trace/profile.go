package trace

import (
	"fmt"
	"math"

	util "github.com/eth-easl/loader/internal"
	"github.com/montanaflynn/stats"
)

const (
	gatewayUrl = "192.168.1.240.sslip.io" // Address of the load balancer.
	namespace  = "default"
	port       = "80"
)

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

type FunctionInvocationStats struct {
	average int
	count   int
	median  int
	minimum int
	maximum int
}

type Function struct {
	name            string
	url             string
	appHash         string
	hash            string
	deployed        bool
	invocationStats FunctionInvocationStats
	durationStats   FunctionDurationStats
	memoryStats     FunctionMemoryStats
}

type FunctionTraces struct {
	path                       string
	Functions                  []Function
	InvocationsPerMinute       [][]int
	TotalInvocationsEachMinute []int
}

const (
	MAX_CONCURRENCY = 100
	MIN_CONCURRENCY = 2
)

func (f *Function) GetExpectedConcurrency() int {
	expectedRps := f.invocationStats.average / 60
	expectedFinishingRatePerSec := float64(f.durationStats.percentile99) / 1000
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

func ProfileFunctionInvocations(funcIdx int, invocations []int) Function {
	data := stats.LoadRawData(invocations)
	median, _ := stats.Median(data)
	median, _ = stats.Round(median, 0)
	max, _ := stats.Max(data)
	min, _ := stats.Min(data)
	count, _ := stats.Sum(data)
	average, _ := stats.Mean(data)

	funcName := fmt.Sprintf("%s-%d", "trace-func", funcIdx)
	return Function{
		name: funcName,
		url:  fmt.Sprintf("%s.%s.%s:%s", funcName, namespace, gatewayUrl, port),
		invocationStats: FunctionInvocationStats{
			average: int(average),
			count:   int(count),
			median:  int(median),
			minimum: int(min),
			maximum: int(max),
		},
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
