package function

import (
	"os/exec"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"

	tc "github.com/eth-easl/loader/internal/trace"
)

func Deploy(functions []tc.Function, serviceConfigPath string, warmupEnabled bool) []tc.Function {
	var urls []string
	deploymentConcurrency := len(functions) //* Fully parallelise deployment.
	sem := make(chan bool, deploymentConcurrency)

	wg := sync.WaitGroup{}
	for idx, function := range functions {
		sem <- true

		go func(function tc.Function, idx int) {
			defer func() {
				<-sem
				wg.Done()
			}()
			wg.Add(1)

			var minScale int
			if minScale = 0; warmupEnabled {
				minScale = function.GetMaxConcurrency()
			}

			log.Info(function.GetName(), " -> min-scale: ", minScale)

			has_deployed := deployFunction(&function, serviceConfigPath, minScale)
			function.SetStatus(has_deployed)

			if has_deployed {
				urls = append(urls, function.GetUrl())
			}
			functions[idx] = function // Update function data.
		}(function, idx)
	}
	wg.Wait()

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
