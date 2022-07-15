package function

import (
	"os/exec"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"

	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	MAX_EXEC_TIME_MILLI             = 60e3   // 60s (avg. p96 from Wild).
	MIN_EXEC_TIME_MILLI             = 1      // 1ms (min. billing unit of AWS).
	MAX_MEM_MIB                     = 10_240 // Max. volume from AWS Lambda. 400MiB (max. p90 for the whole App from Wild).
	MIN_MEM_MIB                     = 1
	OVERPROVISION_MEM_RATIO float32 = 0.3 // From Quasar paper.
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

	var cpuRequest float32
	var memoryRequest int
	switch memoryRequest = util.MinOf(MAX_MEM_MIB, util.MaxOf(128, function.MemoryStats.Percentile100)); {
	// GCP conversion: https://cloud.google.com/functions/pricing
	case memoryRequest <= 128:
		cpuRequest = 0.083
	case memoryRequest <= 512:
		cpuRequest = 0.167
	case memoryRequest <= 1024:
		cpuRequest = 0.333
	case memoryRequest <= 2024:
		cpuRequest = 1
	default:
		cpuRequest = 2
	}
	cpuRequest *= 1000

	cmd := exec.Command(
		"bash",
		"./pkg/function/deploy.sh",
		serviceConfigPath,
		function.Name,

		strconv.Itoa(memoryRequest)+"Mi",
		strconv.Itoa(int(cpuRequest))+"m",
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
