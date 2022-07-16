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
	// NODE_CAPACITY    = 100 //* Empirical limit of a single-node cluster (NOT one-worker cluster).
	NODE_CORE_COUNT  = 16
	MIN_WARMUP_SCALE = 0
)

func ComputeFunctionWarmupScales(clusterSize int, functions []tc.Function) []int {
	var scales []int
	totalClusterCapacityMilli := NODE_CORE_COUNT * clusterSize * 1000
	totalCpuRequestMilli := 0

	for _, function := range functions {
		expectedConcurrency := function.ConcurrencySats.Median
		scale := util.MaxOf(MIN_WARMUP_SCALE, int(math.Ceil(expectedConcurrency))) // Round up.
		scales = append(scales, scale)
		totalCpuRequestMilli += scale * function.CpuRequestMilli
	}

	log.Info("Warmup scales (%d ms) <-> Cluster capacity (%d)", totalCpuRequestMilli, totalClusterCapacityMilli)

	if totalCpuRequestMilli > totalClusterCapacityMilli {
		scales = MaxMinAlloc(totalClusterCapacityMilli, scales, functions)
		log.Info("Max-min scales: ", scales)
	}

	// totalScale, _ := stats.Sum(scalesData)
	// log.Info("Total #pods required:\t", totalScale)
	// log.Info("Warmup scales:\t\t", scales)

	// if totalScale > float64(totalClusterCapacityMilli) {
	// 	//* Rescale warmup scales.
	// 	for i := 0; i < len(scales); i++ {
	// 		ratio := float64(scales[i]) / totalScale
	// 		scales[i] = int(float64(totalClusterCapacityMilli) * ratio) //! Round down to prevent resource outage.
	// 	}
	// }
	// scalesData = stats.LoadRawData(scales)
	// totalScale, _ = stats.Sum(scalesData)
	// log.Info("Rescale to:\t", scales)
	// log.Info("Total #pods granted:\t", totalScale)
	return scales
}

func MaxMinAlloc(totalClusterCapacityMilli int, scales []int, functions []tc.Function) []int {
	newScales := make([]int, len(functions))
	quotas := make([]int, len(functions))
	copy(quotas, scales)
	sort.Ints(quotas)
	prevQuota := -1

max_min_alloc:
	for _, quota := range quotas {
		if quota == 0 || quota == prevQuota {
			continue
		}

		totalPerFunctionRequestedMilli := 0
		for i, function := range functions {
			if scales[i]-newScales[i] > 0 {
				totalPerFunctionRequestedMilli += function.CpuRequestMilli
			}
		}

		//* When there's not enough quota to distribute, prioritise the big ones.
		if totalClusterCapacityMilli < totalPerFunctionRequestedMilli {
			for j := 1; j <= len(quotas); j++ {
				maxQuota := quotas[len(quotas)-j]
				for i, desiredQuota := range scales {
					if desiredQuota == maxQuota {
						ration := totalClusterCapacityMilli / functions[i].CpuRequestMilli
						totalClusterCapacityMilli -= functions[i].CpuRequestMilli * ration
						if totalClusterCapacityMilli >= 0 {
							newScales[i] += ration
						} else {
							break max_min_alloc
						}
					}
				}
			}

		}

		for i := 0; i < len(functions); i++ {
			scale := util.MinOf(quota, util.MaxOf(0, scales[i]-newScales[i]))
			totalClusterCapacityMilli -= functions[i].CpuRequestMilli * scale
			if totalClusterCapacityMilli >= 0 {
				newScales[i] += scale
			} else {
				break max_min_alloc
			}
		}
		prevQuota = quota
	}
	// minScale, _ := stats.LoadRawData(newScales).Min()
	// if minScale == 0 {
	// 	log.Fatal("Not enough capacity")
	// }

	return newScales
}

func Warmup(sampleSize int, totalNumPhases int,
	functions []tc.Function, traces tc.FunctionTraces) int {
	nextPhaseStart := 0
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
