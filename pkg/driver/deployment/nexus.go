package deployment

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/go-cmd/cmd"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/workload/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type nexusDeployer struct{}

type nexusDeploymentConfiguration struct {
	IsPartiallyPanic  bool
	EndpointPort      int
	AutoscalingMetric string
	KhalaEndpoints    []string
}

func newNexusDeployer() *nexusDeployer {
	return &nexusDeployer{}
}

func newNexusDeployerConfiguration(cfg *config.Configuration) *nexusDeploymentConfiguration {
	var khalaEndpoints []string
	workerNodeIp := exec.Command("bash", "-c", `kubectl get nodes -o wide -l 'loader-nodetype in (worker, singlenode)' | awk 'NR>1 {print $6}'`)
	out, err := workerNodeIp.Output()
	if err != nil {
		log.Fatalf("Failed to get worker node IPs: %v", err)
	}
	khalaEndpoints = append(khalaEndpoints, strings.Split(strings.TrimSpace(string(out)), "\n")...)

	log.Debugf("Detected Khala endpoints: %v", khalaEndpoints)
	return &nexusDeploymentConfiguration{
		IsPartiallyPanic:  cfg.LoaderConfiguration.IsPartiallyPanic,
		EndpointPort:      cfg.LoaderConfiguration.EndpointPort,
		AutoscalingMetric: cfg.LoaderConfiguration.AutoscalingMetric,
		KhalaEndpoints:    khalaEndpoints,
	}
}

func (*nexusDeployer) Deploy(cfg *config.Configuration) {
	nexusConfig := newNexusDeployerConfiguration(cfg)

	var khalaClients []*proto.KhalaKnativeIntegrationClient

	// connect to all khala endpoints
	for _, khalaEndpoint := range nexusConfig.KhalaEndpoints {
		conn, err := grpc.NewClient(khalaEndpoint+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Errorf("Failed to connect to nexus endpoint %s: %v", khalaEndpoint, err)
			continue
		}
		defer conn.Close()

		client := proto.NewKhalaKnativeIntegrationClient(conn)
		khalaClients = append(khalaClients, &client)
	}

	for i := 0; i < len(cfg.Functions); i++ {
		// deploy to all khala endpoints
		nexusDeploySingleFunction(
			cfg.Functions[i],
			cfg.Functions[i].YAMLPath,
			nexusConfig.IsPartiallyPanic,
			nexusConfig.EndpointPort,
			nexusConfig.AutoscalingMetric,
			khalaClients,
		)
	}

}

func (*nexusDeployer) Clean() {
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

func nexusDeploySingleFunction(function *common.Function, yamlPath string, isPartiallyPanic bool, endpointPort int, autoscalingMetric string, khalaClients []*proto.KhalaKnativeIntegrationClient) bool {
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
		// strconv.Itoa(function.InitialScale),
		strconv.Itoa(0),
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

	// inform nexus of the new function
	wg := sync.WaitGroup{}
	for _, khalaClient := range khalaClients {
		wg.Add(1)
		go func(client *proto.KhalaKnativeIntegrationClient) {
			defer wg.Done()
			_, err := (*client).CreateSnapshot(context.Background(), &proto.CreateSnapshotRequest{
				Workload: function.Name,
			})
			if err != nil {
				log.Errorf("Failed to inform nexus of new function %s: %v", function.Name, err)
			}
		}(khalaClient)
	}
	wg.Wait()
	return true
}
