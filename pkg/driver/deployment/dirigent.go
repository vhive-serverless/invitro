package deployment

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver/clients"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dirigentDeployer struct {
	deployWorkflow bool
}

type dirigentDeploymentConfiguration struct {
	RegistrationServer string
}

func newDirigentDeployer(deployWorkflow bool) *dirigentDeployer {
	return &dirigentDeployer{
		deployWorkflow: deployWorkflow,
	}
}

func newDirigentDeployerConfiguration(cfg *config.Configuration) dirigentDeploymentConfiguration {
	return dirigentDeploymentConfiguration{
		RegistrationServer: cfg.LoaderConfiguration.DirigentControlPlaneIP,
	}
}

func (d *dirigentDeployer) Deploy(cfg *config.Configuration) {
	dirigentConfig := newDirigentDeployerConfiguration(cfg)

	endpoint := ""

	if d.deployWorkflow {
		wfConfigPath := cfg.LoaderConfiguration.WorkflowConfigPath
		if wfConfigPath == "" {
			log.Fatalf("Failed to deploy workflow: no workflow config path specified in config file.")
		}
		wfConfig := config.ReadWorkflowConfig(wfConfigPath)

		dMetadata := cfg.Functions[0].DirigentMetadata
		if dMetadata == nil {
			log.Fatalf("No Dirigent metadata for workflow %s", cfg.Functions[0].Name)
		}
		tmpNumArgs := dMetadata.NumArgs
		tmpNumRets := dMetadata.NumRets

		// deploy workflow functions
		for _, wfFunc := range wfConfig.Functions {
			dMetadata.NumArgs = wfFunc.NumArgs
			dMetadata.NumRets = wfFunc.NumRets
			tmpFunction := &common.Function{
				Name:                wfFunc.FunctionName,
				CPURequestsMilli:    cfg.Functions[0].CPURequestsMilli,  // NOTE: using first function for now as
				MemoryRequestsMiB:   cfg.Functions[0].MemoryRequestsMiB, // those values are the same for all functions
				ColdStartBusyLoopMs: cfg.Functions[0].ColdStartBusyLoopMs,
				DirigentMetadata:    dMetadata,
			}
			deployDirigentFunction(
				tmpFunction,
				wfFunc.FunctionPath,
				dirigentConfig.RegistrationServer,
				cfg.LoaderConfiguration.BusyLoopOnSandboxStartup,
				cfg.LoaderConfiguration.PrepullMode,
			)
			endpoint = tmpFunction.Endpoint
		}
		dMetadata.NumArgs = tmpNumArgs
		dMetadata.NumRets = tmpNumRets

		// deploy workflow (stored as configuration functions)
		compositionNames := deployDirigentWorkflow(
			cfg.Functions[0],
			dirigentConfig.RegistrationServer,
		)
		// create a function for each registered composition
		newFunctions := make([]*common.Function, len(compositionNames))
		for i, compositionName := range compositionNames {
			newFunctions[i] = cfg.Functions[0]
			newFunctions[i].Endpoint = endpoint
			newFunctions[i].Name = compositionName
			newFunctions[i].WorkflowMetadata = &common.WorkflowMetadata{
				InvocationRequest: clients.WorkflowInvocationBody(
					compositionName,
					clients.CreateDandelionRequest(compositionName, wfConfig.Compositions[i].InDataPaths),
				),
			}
		}
		cfg.Functions = newFunctions

	} else {
		wg := &sync.WaitGroup{}
		wg.Add(len(cfg.Functions))

		for i := 0; i < len(cfg.Functions); i++ {
			go func(idx int) {
				defer wg.Done()

				if cfg.Functions[i].DirigentMetadata == nil {
					log.Fatalf("No Dirigent metadata for function %s", cfg.Functions[i].Name)
				}
				deployDirigentFunction(
					cfg.Functions[idx],
					cfg.Functions[idx].DirigentMetadata.Image,
					dirigentConfig.RegistrationServer,
					cfg.LoaderConfiguration.BusyLoopOnSandboxStartup,
					cfg.LoaderConfiguration.PrepullMode,
				)
			}(i)
		}

		wg.Wait()
	}
}

func (*dirigentDeployer) Clean() {}

var registrationClient = &http.Client{
	Timeout: 300 * time.Second, // time for a request to timeout
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 1500 * time.Millisecond, // time to open socket
		}).DialContext,
		IdleConnTimeout:     2 * time.Second, // unused connections from pool expire after
		MaxIdleConns:        2,
		MaxIdleConnsPerHost: 2,
	},
}

var checkClient = &http.Client{
	Timeout: 5 * time.Second, // time for a request to timeout
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 1500 * time.Millisecond, // time to open socket
		}).DialContext,
		IdleConnTimeout:     2 * time.Second, // unused connections from pool expire after
		MaxIdleConns:        2,
		MaxIdleConnsPerHost: 2,
	},
}

func deployDirigentFunction(function *common.Function, imagePath string, controlPlaneAddress string, busyLoopOnColdStart bool, prepullMode string) {
	metadata := function.DirigentMetadata

	if metadata == nil {
		log.Fatalf("No Dirigent metadata for function %s", function.Name)
	}

	payload := url.Values{
		"name":                {function.Name},
		"image":               {imagePath},
		"port_forwarding":     {strconv.Itoa(metadata.Port), metadata.Protocol},
		"scaling_upper_bound": {strconv.Itoa(metadata.ScalingUpperBound)},
		"scaling_lower_bound": {strconv.Itoa(metadata.ScalingLowerBound)},
		"requested_cpu":       {strconv.Itoa(function.CPURequestsMilli)},
		"requested_memory":    {strconv.Itoa(function.MemoryRequestsMiB)},
		"env_vars":            metadata.EnvVars,     // FORMAT: arg1=value1 arg2=value2 ...
		"program_args":        metadata.ProgramArgs, // FORMAT: arg1 arg2 ...
		"prepull_mode":        {prepullMode},
		"num_args":            {strconv.Itoa(metadata.NumArgs)},
		"num_rets":            {strconv.Itoa(metadata.NumRets)},
	}

	if busyLoopOnColdStart {
		payload["iteration_multiplier"] = []string{strconv.Itoa(function.DirigentMetadata.IterationMultiplier)}
		payload["cold_start_busy_loop_ms"] = []string{strconv.Itoa(function.ColdStartBusyLoopMs)}
	}

	log.Debug(payload)

	resp, err := registrationClient.PostForm(fmt.Sprintf("http://%s/", controlPlaneAddress), payload)
	if err != nil {
		log.Error("Failed to register a service with the control plane - ", err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Failed to read response body.")
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Errorf("Got status code %d while registering %s. Body: %s", resp.StatusCode, function.Name, body)
		return
	}

	endpoints := strings.Split(string(body), ";")
	if len(endpoints) == 0 {
		log.Error("Function registration returned no data plane(s).")
		return
	}
	function.Endpoint = endpoints[rand.Intn(len(endpoints))]

	checkForRegistration(controlPlaneAddress, function.Name, prepullMode)
}

func checkForRegistration(controlPlaneAddress, functionName, prepullMode string) {
	if prepullMode == "" || prepullMode == "none" {
		return
	}

	for {
		resp, err := checkClient.Get(fmt.Sprintf("http://%s/check?name=%s", controlPlaneAddress, functionName))
		if err == nil && resp.StatusCode == http.StatusOK {
			log.Debugf("Function registration %s successful.", functionName)
			break
		} else if err != nil {
			log.Errorf("Failed to send check for registration status: %s", err.Error())
		} else if resp.StatusCode == http.StatusNotFound {
			log.Tracef("Function %s not yet registered.", functionName)
		} else {
			log.Errorf("Status code %d when checking service registration.", resp.StatusCode)
		}

		time.Sleep(5 * time.Second)
	}
}

func deployDirigentWorkflow(wf *common.Function, controlPlaneAddress string) []string {
	metadata := wf.DirigentMetadata
	if metadata == nil {
		log.Fatalf("No Dirigent metadata for workflow %s", wf.Name)
	}

	wfDescription, err := os.ReadFile(metadata.Image)
	if err != nil {
		log.Fatalf("Failed to read workflow description file '%s' : %v", metadata.Image, err)
	}
	payload := url.Values{
		"name":     {wf.Name},
		"workflow": {string(wfDescription)},
	}

	resp, err := registrationClient.PostForm(fmt.Sprintf("http://%s/workflow", controlPlaneAddress), payload)
	if err != nil {
		log.Fatalf("Failed to register a workflow with the control plane - %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body.")
	}

	registeredCompositions := strings.Split(string(body), ";")
	if len(registeredCompositions) == 0 {
		log.Fatalf("Workflow registration returned zero registered workflows.")
	}

	return registeredCompositions
}
