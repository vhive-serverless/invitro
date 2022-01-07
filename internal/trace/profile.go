package trace

import "fmt"

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

func (f *Function) GetMaxConcurrency() int {
	return f.invocationStats.maximum / 60
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
