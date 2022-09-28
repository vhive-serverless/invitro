package cmd

import (
	gen "github.com/eth-easl/loader/pkg/common"
	driver2 "github.com/eth-easl/loader/pkg/driver"
	"math"
	"runtime"
	"sort"

	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
	log "github.com/sirupsen/logrus"
)

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
func ComputeFunctionWarmupScales(clusterSize int, functions []tc.Function) []int {
	var scales []int
	totalClusterCapacityMilli := int(float32(runtime.NumCPU()*clusterSize*1000) * (1.0 - SYS_CPU_OVERHEAD_PERCENT))
	totalCpuRequestMilli := 0

	for _, function := range functions {
		expectedConcurrency := function.ConcurrencyStats.Average
		scale := util.MaxOf(MIN_WARMUP_SCALE, int(math.Ceil(expectedConcurrency))) // Round up.
		scales = append(scales, scale)
		totalCpuRequestMilli += scale * function.CpuRequestMilli
	}

	log.Infof("Warmup CPU demand (%d ms) <-> Cluster capacity (%d ms)", totalCpuRequestMilli, totalClusterCapacityMilli)
	log.Info("Warmup scales: ", scales)

	if totalCpuRequestMilli > totalClusterCapacityMilli {
		log.Fatal("The system is going to overload")
		scales = MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
		log.Info("Max-max scales: ", scales)
	}
	return scales
}

/**
 * Rescales the estimated warm-up `scales` by the total scheduling capacity of the cluster.
 *
 * The setup has been tuned in a way that this function is rarely invoked. However, it's here for security
 * reason, because once the scheduling quota is exceeded, the experiments will halt forever since the
 * required warm-up scales of the function cannot be achieved.
 *
 * The machanism of this function is the opposite of the max-min fairness (https://en.wikipedia.org/wiki/Max-min_fairness).
 * It, instead of caters the smallest requests, starts from the larger ones first since those functions need
 * more function instances to start with.
 *
 * For detailed cases, see: `warmup_test.go`.
 */
func MaxMaxAlloc(totalClusterCapacityMilli int, scales []int, functions []tc.Function) []int {
	scalePairs := make(util.PairList, len(scales))
	for i, scale := range scales {
		scalePairs[i] = util.Pair{Key: i, Value: scale}
	}
	sort.Sort(sort.Reverse(scalePairs))

	quotas := make([]int, len(scales))
	copy(quotas, scales)
	sort.Ints(quotas)

	carry := 0
	for j, pair := range scalePairs {
		desiredScale := pair.Value
		function := functions[(pair.Key).(int)]
		isFirst := false

		if carry == 0 && j < len(scalePairs)-1 {
			isFirst = true
			for i := j + 1; i < len(scalePairs); i++ {
				if scalePairs[i].Value == desiredScale {
					carry++
				} else {
					break
				}
			}
		} else {
			carry--
		}

		totalPossible := totalClusterCapacityMilli / function.CpuRequestMilli
		ration := util.MinOf(desiredScale, totalPossible)
		if isFirst && carry > 0 {
			if totalPossible < desiredScale*(carry+1) {
				ration = totalPossible / (carry + 1)
			}
		} else if carry > 0 {
			ration = scalePairs[j-1].Value
		}

		totalClusterCapacityMilli -= ration * function.CpuRequestMilli
		if totalClusterCapacityMilli >= 0 {
			scalePairs[j].Value = ration
		} else {
			break
		}
	}

	newScales := make([]int, len(scales))
	for _, pair := range scalePairs {
		newScales[(pair.Key).(int)] = pair.Value
	}
	return newScales
}

/**
 * Carries out the warm-up process.
 */
func Warmup(
	sampleSize int,
	totalNumPhases int,
	functions []tc.Function,
	traces tc.FunctionTraces,
	iatDistribution gen.IatDistribution,
	withTracing bool,
	seed int64,
) int {
	//* Skip the profiling minutes.
	nextPhaseStart := gen.PROFILING_DURATION_MINUTES
	for phaseIdx := 1; phaseIdx < totalNumPhases; phaseIdx++ {
		log.Infof("Enter Phase %d as of Minute[%d]", phaseIdx, nextPhaseStart)

		traceLoadParams := driver2.TraceGeneratorParams{
			SampleSize:                    sampleSize,
			PhaseIdx:                      phaseIdx,
			PhaseOffset:                   nextPhaseStart,
			WithBlocking:                  false, //! Non-blocking: directly go to the next phase.
			Functions:                     functions,
			InvocationsEachMinute:         traces.InvocationsEachMinute[nextPhaseStart:],
			TotalNumInvocationsEachMinute: traces.TotalInvocationsPerMinute[nextPhaseStart:],
			IATDistribution:               iatDistribution,
			WithTracing:                   withTracing,
			Seed:                          seed,
		}

		driver := driver2.NewDriver()
		nextPhaseStart = driver.GenerateTraceLoads(traceLoadParams)
	}

	return nextPhaseStart
}

/**
 * The following functions are commented out, because they are not in use
 * but could be useful in the future.
 */
// func setKnConfigMap(patchFilePath string) {
// 	cmd := exec.Command(
// 		"kubectl",
// 		"patch",
// 		"--type=merge",
// 		"configmap",
// 		"config-autoscaler",
// 		"-n",
// 		"knative-serving",
// 		"--patch-file",
// 		patchFilePath,
// 	)
// 	stdoutStderr, err := cmd.CombinedOutput()
// 	log.Info("CMD response: ", string(stdoutStderr))
// 	util.Check(err)
// }

// func livePatchKpas(scriptPath string) {
// 	cmd := exec.Command("bash", scriptPath)
// 	stdoutStderr, err := cmd.CombinedOutput()
// 	log.Info("CMD response: ", string(stdoutStderr))
// 	util.Check(err)
// }

func GetPhasePartitions(collectionLen, partitionSize int) chan IdxRange {
	c := make(chan IdxRange)
	if partitionSize <= 0 {
		close(c)
		return c
	}

	go func() {
		numFullPartitions := collectionLen / partitionSize
		var i int
		for ; i < numFullPartitions; i++ {
			c <- IdxRange{Start: i * partitionSize, End: (i + 1) * partitionSize}
		}

		if collectionLen%partitionSize != 0 { // left over
			c <- IdxRange{Start: i * partitionSize, End: collectionLen}
		}

		close(c)
	}()
	return c
}

type IdxRange struct {
	Start int
	End   int
}
