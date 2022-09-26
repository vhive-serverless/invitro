package trace

import (
	"fmt"
)

//* A bit of a heck to get around cyclic import.
type FunctionSpecsGen func(Function) (int, int)

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
}

type FunctionTraces struct {
	Path                      string
	Functions                 []Function
	WarmupScales              []int
	InvocationsEachMinute     [][]int
	TotalInvocationsPerMinute []int
}

func (f *Function) SetHash(hash int) {
	f.HashFunction = fmt.Sprintf("%015d", hash)
}

func (f *Function) SetName(name string) {
	f.Name = name
}

func (f *Function) SetStatus(b bool) {
	f.Deployed = b
}

func (f *Function) GetStatus() bool {
	return f.Deployed
}

func (f *Function) GetName() string {
	return f.Name
}

func (f *Function) GetUrl() string {
	return f.Endpoint
}

func (f *Function) SetUrl(url string) {
	f.Endpoint = url
}
