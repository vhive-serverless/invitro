package function

import (
	"os/exec"
	"strconv"

	log "github.com/sirupsen/logrus"

	tc "github.com/eth-easl/loader/pkg/trace"
)

func DeployFunctions(functions []tc.Function, serviceConfigPath string, initScales []int) []tc.Function {
	var urls []string
	// deploymentConcurrency := 1 //* Serialise deployment.
	deploymentConcurrency := len(functions) //* Fully parallelise deployment.
	sem := make(chan bool, deploymentConcurrency)

	for funcIdx, function := range functions {
		sem <- true
		go func(function tc.Function, funcIdx int) {
			defer func() { <-sem }()

			var initScale int
			if len(initScales) == len(functions) {
				initScale = initScales[funcIdx]
			} else {
				initScale = 0 //* No-warmup (`initScales` is not populated).
			}
			// log.Info(function.GetName(), " -> initScale: ", initScale)

			hasDeployed := deploy(&function, serviceConfigPath, initScale)
			function.SetStatus(hasDeployed)

			if hasDeployed {
				urls = append(urls, function.GetUrl())
			}
			functions[funcIdx] = function // Update function data.
		}(function, funcIdx)
	}
	//* Block until all slots are refilled (after they have all been consumed).
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	// defer CreateGrpcPool(functions)
	return functions
}

func deploy(function *tc.Function, serviceConfigPath string, initScale int) bool {
	cmd := exec.Command(
		"kn",
		"service",
		"apply",
		function.Name,
		"-f",
		serviceConfigPath,
		//! `--scale-min` should NOT be used here since it only has a per-revision key,
		//! i.e., it will change the min scale for all the (future) pods of this kn service.
		"--scale-init",
		strconv.Itoa(initScale),
		"--concurrency-target",
		"1",
		//* Wait for infintely long for ensuring warmup.
		"--wait-timeout",
		"2000000",
	)
	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.GetName(), err, stdoutStderr)
		return false
	}

	log.Info("Deployed function ", function.Endpoint)
	return true
}
