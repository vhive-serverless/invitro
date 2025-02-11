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
	"time"
)

type dirigentDeployer struct {
	deployWorkflow bool
}

type dirigentDeploymentConfiguration struct {
	RegistrationServer string
	ColdStartSweep     bool
}

func newDirigentDeployer(deployWorkflow bool) *dirigentDeployer {
	return &dirigentDeployer{
		deployWorkflow: deployWorkflow,
	}
}

func newDirigentDeployerConfiguration(cfg *config.Configuration) dirigentDeploymentConfiguration {
	return dirigentDeploymentConfiguration{
		RegistrationServer: cfg.LoaderConfiguration.DirigentControlPlaneIP,
		ColdStartSweep:     int(cfg.LoaderConfiguration.RpsTarget) != 0,
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
				dirigentConfig.ColdStartSweep,
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
		for i := 0; i < len(cfg.Functions); i++ {
			if cfg.Functions[i].DirigentMetadata == nil {
				log.Fatalf("No Dirigent metadata for function %s", cfg.Functions[i].Name)
			}
			deployDirigentFunction(
				cfg.Functions[i],
				cfg.Functions[i].DirigentMetadata.Image,
				dirigentConfig.RegistrationServer,
				cfg.LoaderConfiguration.BusyLoopOnSandboxStartup,
				dirigentConfig.ColdStartSweep,
			)
		}
	}
}

func (*dirigentDeployer) Clean() {}

var registrationClient = &http.Client{
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

func deployDirigentFunction(function *common.Function, imagePath string, controlPlaneAddress string, busyLoopOnColdStart bool, coldStartSweep bool) {
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
		"cold_start_sweep":    {strconv.FormatBool(coldStartSweep)},
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

	endpoints := strings.Split(string(body), ";")
	if len(endpoints) == 0 {
		log.Error("Function registration returned no data plane(s).")
		return
	}
	function.Endpoint = endpoints[rand.Intn(len(endpoints))]
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

	resp, err := registrationClient.PostForm("http://"+controlPlaneAddress+"/workflow", payload)
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
