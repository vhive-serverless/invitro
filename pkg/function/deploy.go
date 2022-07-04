package function

import (
	"os/exec"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"

	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
)

var (
	regex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

func DeployFunctions(
	functions []tc.Function,
	serviceConfigPath string,
	initScales []int,
	isPartiallyPanic bool,
) []tc.Function {
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

			hasDeployed := deploy(&function, serviceConfigPath, initScale, isPartiallyPanic)
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

func deploy(function *tc.Function, serviceConfigPath string, initScale int, isPartiallyPanic bool) bool {
	panicWindow := "\"10.0\""
	panicThreshold := "\"200.0\""
	if isPartiallyPanic {
		panicWindow = "\"100.0\""
		panicThreshold = "\"1000.0\""
	}

	memoryLimit := util.MinOf(128, function.MemoryStats.Percentile99)
	cpuLimit := 1000 * memoryLimit / 1_769

	cmd := exec.Command(
		"bash",
		"./pkg/function/deploy.sh",
		serviceConfigPath,
		function.Name,

		strconv.Itoa(memoryLimit)+"Mi",
		strconv.Itoa(cpuLimit)+"m",
		strconv.Itoa(initScale),

		panicWindow,
		panicThreshold,
	)

	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.Name, err, stdoutStderr)
		return false
	}

	if endpoint := regex.FindStringSubmatch(string(stdoutStderr))[1]; endpoint != function.Endpoint {
		log.Warnf("Update function endpoint to %s\n", endpoint)
		function.Endpoint = endpoint
	}

	log.Info("Deployed function ", function.Endpoint)
	return true
}
