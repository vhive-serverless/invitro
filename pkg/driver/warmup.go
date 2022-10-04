package driver

import (
	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
	"math"
	"runtime"
)

// TODO: this file has not been yet reviewed

const (
	// Miminimum warm-up scale.
	MIN_WARMUP_SCALE = 0
	// Percentage of system overheads taken up by the kernel and other components (e.g., prometheus)
	// in the scheduling quota.
	SYS_CPU_OVERHEAD_PERCENT float32 = 0.6
)

/**
 * Computes the estimated number of instances each function needs.
 *
 * The calculation is based on the profiled concurrency (see the `trace` package) using Little's law.
 */
func ComputeFunctionWarmupScales(clusterSize int, functions []*common.Function) []int {
	var scales []int
	totalClusterCapacityMilli := int(float32(runtime.NumCPU()*clusterSize*1000) * (1.0 - SYS_CPU_OVERHEAD_PERCENT))
	totalCpuRequestMilli := 0

	for _, function := range functions {
		expectedConcurrency := function.StaticProfilingConcurrency
		scale := common.MaxOf(MIN_WARMUP_SCALE, int(math.Ceil(expectedConcurrency))) // Round up.
		scales = append(scales, scale)
		totalCpuRequestMilli += scale * function.CpuRequestMilli
	}

	log.Infof("Warmup CPU demand (%d ms) <-> Cluster capacity (%d ms)", totalCpuRequestMilli, totalClusterCapacityMilli)
	log.Info("Warmup scales: ", scales)

	if totalCpuRequestMilli > totalClusterCapacityMilli {
		log.Fatal("The system is going to overload")
		//scales = MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
		log.Info("Max-max scales: ", scales)
	}
	return scales
}
