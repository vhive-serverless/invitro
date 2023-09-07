package driver

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/eth-easl/loader/pkg/common"

	log "github.com/sirupsen/logrus"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

var (
	urlRegex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

func DeployFunctionsKnative(functions []*common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string, CPUScheduler string) {
	for i := 0; i < len(functions); i++ {
		deployOne(functions[i], yamlPath, isPartiallyPanic, endpointPort, autoscalingMetric, CPUScheduler)
	}
}

func deployOne(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string, CPUScheduler string) bool {
	panicWindow := "\"10.0\""
	panicThreshold := "\"200.0\""
	if isPartiallyPanic {
		panicWindow = "\"100.0\""
		panicThreshold = "\"1000.0\""
	}
	autoscalingTarget := 100 // default for concurrency
	if autoscalingMetric == "rps" {
		autoscalingTarget = int(math.Round(1000.0 / function.RuntimeStats.Average))
		// for rps mode use the average runtime in milliseconds to determine how many requests a pod can process per
		// second, then round to an integer as that is what the knative config expects
	}
	var cpuRequest int
	var cpuLimit int
	if CPUScheduler == "memory" {
		cpuRequest = function.CPURequestsMilli
		cpuLimit = function.CPULimitsMilli
	} else if CPUScheduler == "runtime" {
		var cpu float64
		switch runtime := function.RuntimeStats.Average; {
		case runtime < 100:
			cpu = 1
		case runtime < 300:
			cpu = 0.9
		case runtime < 500:
			cpu = 0.8
		case runtime < 750:
			cpu = 0.7
		case runtime < 1000:
			cpu = 0.6
		default:
			cpu = 0.5
		}
		cpuRequest = int(cpu * 1000)
		cpuLimit = 1000
	} else {
		cpuRequest = 100
		cpuLimit = 1000
	}

	cmd := exec.Command(
		"bash",
		"./pkg/driver/deploy.sh",
		yamlPath,
		function.Name,

		strconv.Itoa(cpuRequest)+"m",
		strconv.Itoa(cpuLimit)+"m",
		strconv.Itoa(function.MemoryRequestsMiB)+"Mi",
		strconv.Itoa(function.InitialScale),

		panicWindow,
		panicThreshold,

		"\""+autoscalingMetric+"\"",
		"\""+strconv.Itoa(autoscalingTarget)+"\"",
	)

	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))

	if err != nil {
		// TODO: there should be a toggle to turn off deployment because if this is fatal then we cannot test the thing locally
		log.Warnf("Failed to deploy function %s: %v\n%s\n", function.Name, err, stdoutStderr)

		return false
	}

	if endpoint := urlRegex.FindStringSubmatch(string(stdoutStderr))[1]; endpoint != function.Endpoint {
		// TODO: check when this situation happens
		log.Debugf("Update function endpoint to %s\n", endpoint)
		function.Endpoint = endpoint
	} else {
		function.Endpoint = fmt.Sprintf("%s.%s.%s", function.Name, namespace, bareMetalLbGateway)
	}

	// adding port to the endpoint
	function.Endpoint = fmt.Sprintf("%s:%d", function.Endpoint, endpointPort)

	log.Debugf("Deployed function on %s\n", function.Endpoint)
	return true
}

func CleanKnative() {
	cmd := exec.Command("kn", "service", "delete", "--all")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Debugf("Unable to delete Knative services - %s", err)
	}
}
