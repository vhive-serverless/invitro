package deployment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

var (
	urlRegex = regexp.MustCompile("at URL:\nhttp://([^\n]+)")
)

type knativeDeployer struct{}

type knativeDeploymentConfiguration struct {
	YamlPath          string
	IsPartiallyPanic  bool
	EndpointPort      int
	AutoscalingMetric string
}

func newKnativeDeployer() *knativeDeployer {
	return &knativeDeployer{}
}

func newKnativeDeployerConfiguration(cfg *config.Configuration) knativeDeploymentConfiguration {
	return knativeDeploymentConfiguration{
		YamlPath:          cfg.YAMLPath,
		IsPartiallyPanic:  cfg.LoaderConfiguration.IsPartiallyPanic,
		EndpointPort:      cfg.LoaderConfiguration.EndpointPort,
		AutoscalingMetric: cfg.LoaderConfiguration.AutoscalingMetric,
	}
}

func (*knativeDeployer) Deploy(cfg *config.Configuration) {
	knativeConfig := newKnativeDeployerConfiguration(cfg)

	for i := 0; i < len(cfg.Functions); i++ {
		knativeDeploySingleFunction(
			cfg.Functions[i],
			knativeConfig.YamlPath,
			knativeConfig.IsPartiallyPanic,
			knativeConfig.EndpointPort,
			knativeConfig.AutoscalingMetric,
			cfg.LoaderConfiguration.VSwarm,
		)
	}
}

func (*knativeDeployer) Clean() {
	cmd := exec.Command("kn", "service", "delete", "--all")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Errorf("Unable to delete Knative services - %s", err)
	}
}

func knativeDeploySingleFunction(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int,
	autoscalingMetric string, vSwarm bool) bool {
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

	// Default is the function name, but if the function is a vSwarm function, the proxy name is used
	proxyName := function.Name

	if vSwarm {
		// Read and unmarshal the mapper output file into a map
		
		mapperFile, err := os.ReadFile("tools/mapper/output.json")
		if err != nil {
			log.Warn("No mapper output file")
		}
		var mapperOutput map[string]map[string]string		// HashFunction mapped to vSwarm function yaml.
		json.Unmarshal(mapperFile, &mapperOutput)

		// Find the proxy name for the function
		proxyName = mapperOutput[function.HashFunction]["proxy-function"]
		log.Info("Proxy name: ", proxyName)
		// Read the deployment info file for yaml locations and predeployment commands if any

		deploymentInfoFile, err := os.ReadFile(yamlPath + "deploy_info.json")
		if err != nil {
			log.Warn("No deployment info file")
		}

		var deploymentInfo map[string]map[string]interface{}

		json.Unmarshal(deploymentInfoFile, &deploymentInfo)

		// Get the yaml location and predeployment commands for the function
		yamlLocation := deploymentInfo[proxyName]["yaml-location"].(string)
		// Modify the yaml location to drop the leading "./" and add the yaml path
		yamlPath = yamlLocation
		// Get the list of predeployment commands
		predeploymentCommands := deploymentInfo[proxyName]["predeployment-commands"].([]interface{}) 

		// Run the predeployment commands
		for _, command := range predeploymentCommands {
			cmd := exec.Command("bash", "-c", command.(string))
			stdoutStderr, err := cmd.CombinedOutput()
			log.Debug("Predeployment command response: ", string(stdoutStderr))
			if err != nil {
				log.Warnf("Failed to run predeployment command %s: %v\n%s\n", command, err, stdoutStderr)
				return false
			}
		}
	}

	cmd := exec.Command(
		"bash",
		"./pkg/driver/deployment/knative.sh",
		yamlPath,
		proxyName,

		strconv.Itoa(function.CPURequestsMilli)+"m",
		strconv.Itoa(function.CPULimitsMilli)+"m",
		strconv.Itoa(function.MemoryRequestsMiB)+"Mi",
		strconv.Itoa(function.InitialScale),
		panicWindow,
		panicThreshold,

		wrapString(autoscalingMetric),
		wrapString(strconv.Itoa(autoscalingTarget)),

		wrapString(strconv.Itoa(function.ColdStartBusyLoopMs)),
	)

	stdoutStderr, err := cmd.CombinedOutput()
	log.Debug("CMD response: ", string(stdoutStderr))
	// Wait for 2 seconds to allow the function to be deployed

	cmd = exec.Command("sleep", "2")
	cmd.Run()

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

func wrapString(value string) string {
	return "\"" + value + "\""
}
