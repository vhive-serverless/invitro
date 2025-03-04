package deployment

import (
	"bytes"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"sync"

	"github.com/go-cmd/cmd"
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
	IsPartiallyPanic  bool
	EndpointPort      int
	AutoscalingMetric string
}

func newKnativeDeployer() *knativeDeployer {
	return &knativeDeployer{}
}

func newKnativeDeployerConfiguration(cfg *config.Configuration) knativeDeploymentConfiguration {
	return knativeDeploymentConfiguration{
		IsPartiallyPanic:  cfg.LoaderConfiguration.IsPartiallyPanic,
		EndpointPort:      cfg.LoaderConfiguration.EndpointPort,
		AutoscalingMetric: cfg.LoaderConfiguration.AutoscalingMetric,
	}
}

func (*knativeDeployer) Deploy(cfg *config.Configuration) {
	knativeConfig := newKnativeDeployerConfiguration(cfg)

	queue := make(chan struct{}, runtime.NumCPU()) // message queue as a sync method
	deployed := sync.WaitGroup{}
	deployed.Add(len(cfg.Functions))

	for i := 0; i < len(cfg.Functions); i++ {
		go func() {
			queue <- struct{}{}

			defer deployed.Done()
			defer func() { <-queue }()

			knativeDeploySingleFunction(
				cfg.Functions[i],
				cfg.Functions[i].YAMLPath,
				knativeConfig.IsPartiallyPanic,
				knativeConfig.EndpointPort,
				knativeConfig.AutoscalingMetric,
			)
		}()
	}

	deployed.Wait()
}

func (*knativeDeployer) Clean() {
	cmd := exec.Command("kn", "service", "delete", "--all")

	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		log.Errorf("Unable to delete Knative services - %s", err)
	}
	preDepCmd := exec.Command("kubectl", "delete", "pods", "--all")
	preDepCmd.Stdout = &out
	if err := preDepCmd.Run(); err != nil {
		log.Error("Unable to clean up predeployment files")
	}
	preDepCmd = exec.Command("kubectl", "delete", "services", "--all")
	preDepCmd.Stdout = &out
	if err := preDepCmd.Run(); err != nil {
		log.Error("Unable to clean up predeployment files")
	}
}

func knativeDeploySingleFunction(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int, autoscalingMetric string) bool {
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
	for _, path := range function.PredeploymentPath {
		envCmd := cmd.NewCmd("kubectl", "apply", "-f", path)
		status := <-envCmd.Start()

		for _, line := range status.Stdout {
			fmt.Println("Predeployment command response is" + line)
		}
	}
	cmd := exec.Command(
		"bash",
		"./pkg/driver/deployment/knative.sh",
		yamlPath,
		function.Name,

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
