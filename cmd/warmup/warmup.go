package cmd

import (
	"os/exec"

	util "github.com/eth-easl/loader/internal"
	log "github.com/sirupsen/logrus"
)

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
	log.Debug("CMD response: ", string(stdoutStderr))
	util.Check(err)
}

func LivePatchKpas(scriptPath string) {
	cmd := exec.Command("bash", scriptPath)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))
	util.Check(err)
}

func ComputePhasePartition(collectionLen, partitionSize int) chan IdxRange {
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
