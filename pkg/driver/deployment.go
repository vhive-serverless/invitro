package driver

import (
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"

	log "github.com/sirupsen/logrus"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

var (
	urlRegex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

func min(nums ...int) int {
	if len(nums) == 0 {
		return 0
	}
	min := nums[0]
	for _, num := range nums {
		if num < min {
			min = num
		}
	}

	return min
}

func DeployFunctions(loaderConfiguration *config.LoaderConfiguration, functions []*common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string) {
	for i := 0; i < len(functions); i++ {
		if IsStringInList(loaderConfiguration.ClientTraining, []string{common.Batch, common.BatchPriority, common.PipelineBatchPriority, common.GradientAccumulation, common.ServerfulOptimus}) {
			deployOne(functions[i], yamlPath, isPartiallyPanic, endpointPort, autoscalingMetric)
		} else if IsStringInList(loaderConfiguration.ClientTraining, []string{common.Multi, common.HiveD, common.HiveDElastic, common.Elastic}) {
			// loaderConfiguration.ClientTraining == common.Single || loaderConfiguration.ClientTraining == common.HiveD {

			parts := strings.Split(functions[i].Name, "-")
			gpuCount, err := strconv.Atoi(parts[len(parts)-1])
			if err == nil {
				fmt.Println(gpuCount, functions[i].Name)
			}
			deployOneWithGPU(gpuCount, functions[i], yamlPath, isPartiallyPanic, endpointPort, autoscalingMetric)
		} else {
			log.Errorf("Invalid client_training value: %s", loaderConfiguration.ClientTraining)
		}
	}
	for i := 0; i < len(functions); i++ {
		log.Infof("DepolyFunctions: Name[%s], Port [%s]", functions[i].Name, functions[i].Endpoint)
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
	fmt.Println(yamlPath)
	cmd := exec.Command(
		"bash",
		"./pkg/driver/deploy_gpt.sh",
		yamlPath,
		function.Name,

		strconv.Itoa(function.CPURequestsMilli)+"m",
		strconv.Itoa(function.CPULimitsMilli)+"m",
		strconv.Itoa(function.MemoryRequestsMiB)+"Mi",
		"1",
		"1",
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

func deployOneWithGPU(gpuCount int, function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
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
	extra_resource_scale := min(gpuCount, 8)
	cmd := exec.Command(
		"bash",
		"./pkg/driver/deploy_gpt.sh",
		yamlPath,
		function.Name,

		strconv.Itoa(function.CPURequestsMilli*extra_resource_scale)+"m",
		strconv.Itoa(function.CPULimitsMilli*extra_resource_scale)+"m",
		strconv.Itoa(function.MemoryRequestsMiB*extra_resource_scale)+"Mi",
		fmt.Sprintf("%d", gpuCount),
		fmt.Sprintf("%d", gpuCount),
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

func DeployPromptFunctions(loaderConfiguration *config.LoaderConfiguration, functions []*common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string) []*common.Function {
	promptFunctions := make([]*common.Function, len(functions))
	for i := 0; i < len(functions); i++ {
		copy := *functions[i] // make a copy of the function
		copy.Name = fmt.Sprintf("promptbank-%s", functions[i].Name)
		promptFunctions[i] = &copy

		if IsStringInList(loaderConfiguration.ClientTraining, []string{common.Batch, common.BatchPriority, common.PipelineBatchPriority}) {
			deployPromptBankOne(promptFunctions[i], loaderConfiguration.PromptYamlPath, isPartiallyPanic, endpointPort, autoscalingMetric)
		} else {
			log.Errorf("Invalid client_training value: %s", loaderConfiguration.ClientTraining)
		}
	}
	for i := 0; i < len(promptFunctions); i++ {
		log.Infof("DepolyFunctions: Name[%s], Port [%s]", promptFunctions[i].Name, promptFunctions[i].Endpoint)
	}
	return promptFunctions
}

func deployPromptBankOne(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
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
		"./pkg/driver/deploy_prompt_bank.sh",
		yamlPath,
		function.Name,

		strconv.Itoa(function.CPURequestsMilli)+"m",
		strconv.Itoa(function.CPULimitsMilli)+"m",
		strconv.Itoa(function.MemoryRequestsMiB)+"Mi",
		"1",
		"1",
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
