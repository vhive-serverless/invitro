package cmd

import (
	"math"
	"os/exec"
	"sort"

	util "github.com/eth-easl/loader/pkg"
	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
	log "github.com/sirupsen/logrus"
)

const (
	NODE_CORE_COUNT                  = 16
	MIN_WARMUP_SCALE                 = 0
	SYS_CPU_OVERHEAD_PERCENT float32 = 0.6
)

func ComputeFunctionWarmupScales(clusterSize int, functions []tc.Function) []int {
	var scales []int
	totalClusterCapacityMilli := int(float32(NODE_CORE_COUNT*clusterSize*1000) * (1.0 - SYS_CPU_OVERHEAD_PERCENT))
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
		scales = MaxMaxAlloc(totalClusterCapacityMilli, scales, functions)
		log.Info("Max-max scales: ", scales)
	}
	return scales
}

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

func Warmup(
	sampleSize int,
	totalNumPhases int,
	functions []tc.Function,
	traces tc.FunctionTraces,
) int {
	//* Skip the profiling minutes.
	nextPhaseStart := gen.PROFILING_DURATION_MINUTES
	for phaseIdx := 1; phaseIdx < totalNumPhases; phaseIdx++ {
		// //* Set up kn environment
		// if phaseIdx == 1 {
		// 	setKnConfigMap("config/kn_configmap_warmup_init_patch.yaml")
		// }

		log.Infof("Enter Phase %d as of Minute[%d]", phaseIdx, nextPhaseStart)
		nextPhaseStart = gen.GenerateTraceLoads(
			sampleSize,
			phaseIdx,
			nextPhaseStart,
			false, //! Non-blocking: directly go to the next phase.
			functions,
			traces.InvocationsEachMinute[nextPhaseStart:],
			traces.TotalInvocationsPerMinute[nextPhaseStart:],
		)

		// //* Reset kn environment
		// if phaseIdx == 1 {
		// 	setKnConfigMap("config/kn_configmap_warmup_reset_patch.yaml")
		// 	livePatchKpas("scripts/warmup/livepatch_kpa.sh")
		// }
	}
	return nextPhaseStart
}

func setKnConfigMap(patchFilePath string) {
	cmd := exec.Command(
		"kubectl",
		"patch",
		"--type=merge",
		"configmap",
		"config-autoscaler",
		"-n",
		"knative-serving",
		"--patch-file",
		patchFilePath,
	)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Info("CMD response: ", string(stdoutStderr))
	util.Check(err)
}

func livePatchKpas(scriptPath string) {
	cmd := exec.Command("bash", scriptPath)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Info("CMD response: ", string(stdoutStderr))
	util.Check(err)
}

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
