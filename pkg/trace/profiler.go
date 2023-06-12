package trace

import (
	"math"

	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
)

func DoStaticTraceProfiling(functions []*common.Function) {
	for i := 0; i < len(functions); i++ {
		f := functions[i]

		f.InitialScale = int(math.Ceil(profileConcurrency(functions[i])))
		log.Debugf("Function %s initial scale will be %d.\n", f.Name, f.InitialScale)
	}
}

func ApplyResourceLimits(functions []*common.Function) {
	for i := 0; i < len(functions); i++ {
		memoryPct100 := int(functions[i].MemoryStats.Percentile100)
		cpuShare := ConvertMemoryToCpu(memoryPct100)

		functions[i].CPURequestsMilli = cpuShare / common.OvercommitmentRatio
		functions[i].MemoryRequestsMiB = memoryPct100 / common.OvercommitmentRatio
		functions[i].CPULimitsMilli = cpuShare
	}
}

// ConvertMemoryToCpu Google Cloud Function conversion table used from https://cloud.google.com/functions/pricing
func ConvertMemoryToCpu(memoryRequest int) int {
	var cpuRequest float32
	switch memoryRequest = common.MinOf(common.MaxMemQuotaMib, common.MaxOf(common.MinMemQuotaMib, memoryRequest)); {
	case memoryRequest < 256:
		cpuRequest = 0.083
	case memoryRequest < 512:
		cpuRequest = 0.167
	case memoryRequest < 1024:
		cpuRequest = 0.333
	case memoryRequest < 2048:
		cpuRequest = 0.583
	case memoryRequest < 4096:
		cpuRequest = 1
	default:
		cpuRequest = 2
	}

	return int(cpuRequest * 1000)
}

func profileConcurrency(function *common.Function) float64 {
	IPM := function.InvocationStats.Invocations[0]

	// Arrival rate - unit 1 s
	rps := float64(IPM) / 60.0
	// Processing rate = runtime_in_milli / 1000, assuming it can be process right away upon arrival.
	processingRate := float64(function.RuntimeStats.Average) / 1000.0
	// Expected concurrency == the inventory (total #jobs in the system) of Little's law.
	concurrency := rps * processingRate

	return concurrency
}
