package function

import (
	"os/exec"
	"strconv"

	log "github.com/sirupsen/logrus"

	tc "github.com/eth-easl/loader/internal/trace"
)

func Deploy(functions []tc.Function, serviceConfigPath string, minScales []int) []tc.Function {
	log.Info("Using service config file: ", serviceConfigPath)
	var urls []string
	deploymentConcurrency := 1 //* Serialise deployment.
	// deploymentConcurrency := len(functions) //* Fully parallelise deployment.
	sem := make(chan bool, deploymentConcurrency)

	for funcIdx, function := range functions {
		sem <- true
		go func(function tc.Function, funcIdx int) {
			defer func() { <-sem }()

			var minScale int
			if len(minScales) == len(functions) {
				minScale = minScales[funcIdx]
			} else {
				minScale = 0 //* No-warmup (`minScales` is not populated).
			}
			// log.Info(function.GetName(), " -> minScale: ", minScale)

			has_deployed := deployFunction(&function, serviceConfigPath, minScale)
			function.SetStatus(has_deployed)

			if has_deployed {
				urls = append(urls, function.GetUrl())
			}
			functions[funcIdx] = function // Update function data.
		}(function, funcIdx)
	}
	//* Block until all slots are refilled (after they have all been consumed).
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
	return functions
}

func deployFunction(function *tc.Function, workloadPath string, minScale int) bool {
	//TODO: Make concurrency configurable.
	cmd := exec.Command(
		"kn",
		"service",
		"apply",
		function.GetName(),
		"-f",
		workloadPath,
		"--scale-min",
		strconv.Itoa(minScale),
		"--concurrency-target",
		"1",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.GetName(), err, stdoutStderr)
		return false
	}

	log.Info("Deployed function ", function.GetUrl())
	return true
}
