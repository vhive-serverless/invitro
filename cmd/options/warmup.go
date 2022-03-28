package cmd

import (
	"math"
	"os/exec"

	util "github.com/eth-easl/loader/pkg"
	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
)

const (
	NODE_CAPACITY    = 160 //* Empirical limit of a single-node cluster (NOT one-worker cluster).
	MIN_WARMUP_SCALE = 2
)

func ComputeFunctionsWarmupScales(clusterSize int, functions []tc.Function) []int {
	var scales []int
	totalCapacity := NODE_CAPACITY * clusterSize

	for _, function := range functions {
		expectedConcurrency := function.ConcurrencySats.Median
		scale := util.MaxOf(MIN_WARMUP_SCALE, int(math.Ceil(expectedConcurrency))) //* Round up.
		scales = append(scales, scale)
	}

	scalesData := stats.LoadRawData(scales)
	totalScale, _ := stats.Sum(scalesData)
	log.Info("Total #pods required:\t", totalScale)
	log.Info("Warmup scales:\t\t", scales)

	if totalScale > float64(totalCapacity) {
		//* Rescale warmup scales.
		for i := 0; i < len(scales); i++ {
			ratio := float64(scales[i]) / totalScale
			scales[i] = int(float64(totalCapacity) * ratio) //! Round down to prevent kn outage.
		}
	}
	scalesData = stats.LoadRawData(scales)
	totalScale, _ = stats.Sum(scalesData)
	log.Info("Rescale to:\t", scales)
	log.Info("Total #pods granted:\t", totalScale)
	return scales
}

func Warmup(sampleSize int, totalNumPhases int, rps int,
	functions []tc.Function, traces tc.FunctionTraces) int {
	nextPhaseStart := 0
	for phaseIdx := 1; phaseIdx < totalNumPhases; phaseIdx++ {
		//* Set up kn environment
		if phaseIdx == 1 {
			setKnConfigMap("config/kn_configmap_init_patch.yaml")
		}

		log.Infof("Enter Phase %d as of Minute[%d]", phaseIdx, nextPhaseStart)
		nextPhaseStart = gen.GenerateTraceLoads(
			sampleSize,
			phaseIdx,
			nextPhaseStart,
			false, //! Non-blocking: directly go into the next phase.
			rps,
			functions,
			traces.InvocationsEachMinute[nextPhaseStart:],
			traces.TotalInvocationsPerMinute[nextPhaseStart:])

		//* Reset kn environment
		if phaseIdx == 1 {
			setKnConfigMap("config/kn_configmap_reset_patch.yaml")
			livePatchKpas("scripts/warmup/livepatch_kpas.sh")
		}
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
