package driver

import (
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"math"
	"os/exec"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

var (
	urlRegex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

func DeployFunctions(functions []*common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string) {
	for i := 0; i < len(functions); i++ {
		deployOne(functions[i], yamlPath, isPartiallyPanic, endpointPort, autoscalingMetric)
	}
}

func deployOne(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string) bool {
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

	cmd := exec.Command(
		"bash",
		"./pkg/driver/deploy.sh",
		yamlPath,
		function.Name,

		strconv.Itoa(function.CPURequestsMilli)+"m",
		strconv.Itoa(function.CPULimitsMilli)+"m",
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
