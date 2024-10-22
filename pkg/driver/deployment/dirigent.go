package deployment

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
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
		if cfg.Functions[0].DirigentMetadata == nil {
			log.Fatalf("No Dirigent metadata for workflow %s", cfg.Functions[0].Name)
		}

		// deploy workflow functions
		for i := 0; i < len(cfg.LoaderConfiguration.WorkflowFunctionNames); i++ {
			tmpFunction := &common.Function{
				Name:                cfg.LoaderConfiguration.WorkflowFunctionNames[i],
				CPURequestsMilli:    cfg.Functions[0].CPURequestsMilli,  // NOTE: using first function for now as
				MemoryRequestsMiB:   cfg.Functions[0].MemoryRequestsMiB, // those values are the same for all functions
				ColdStartBusyLoopMs: cfg.Functions[0].ColdStartBusyLoopMs,
				DirigentMetadata:    cfg.Functions[0].DirigentMetadata,
			}
			deployDirigentFunction(
				tmpFunction,
				cfg.LoaderConfiguration.WorkflowFunctionPaths[i],
				dirigentConfig.RegistrationServer,
				cfg.LoaderConfiguration.BusyLoopOnSandboxStartup,
				dirigentConfig.ColdStartSweep,
			)
			endpoint = tmpFunction.Endpoint
		}

		// deploy workflows (stored as configuration functions)
		for i := 0; i < len(cfg.Functions); i++ {
			deployDirigentWorkflow(
				cfg.Functions[i],
				dirigentConfig.RegistrationServer,
			)
			cfg.Functions[i].Endpoint = endpoint
		}
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

func deployDirigentWorkflow(wf *common.Function, controlPlaneAddress string) {
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

	registeredWorkflows := strings.Split(string(body), ";")
	if len(registeredWorkflows) != 1 {
		log.Fatalf("Workflow registration returned zero or more than one registered workflows.")
	}

	wf.Name = registeredWorkflows[0]
}
