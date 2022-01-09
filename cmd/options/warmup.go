package cmd

import (
	"math"
	"os/exec"

	util "github.com/eth-easl/loader/internal"
	tc "github.com/eth-easl/loader/internal/trace"
	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
)

const (
	CAPACITY         = 219 //! Sharp
	MIN_WARMUP_SCALE = 2
)

func ComputeFunctionsWarmupScales(functions []tc.Function) []int {
	var scales []int

	for _, function := range functions {
		expectedConcurrency := function.ConcurrencySats.Median
		scale := util.MaxOf(MIN_WARMUP_SCALE, int(math.Ceil(expectedConcurrency))) //* Round up.
		scales = append(scales, scale)
	}

	scalesData := stats.LoadRawData(scales)
	totalScale, _ := stats.Sum(scalesData)
	log.Info("Total #pods required:\t", totalScale)
	log.Info("Warmup scales:\t", scales)

	if totalScale > CAPACITY {
		//* Rescale warmup scales.
		for i := 0; i < len(scales); i++ {
			ratio := float64(scales[i]) / totalScale
			scales[i] = int(float64(CAPACITY) * ratio) //! Round down to prevent kn outage.
		}
		log.Info("Rescale to:\t", scales)
	}
	return scales
}

func SetKnConfigMap(patchFilePath string) {
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

func LivePatchKpas(scriptPath string) {
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
