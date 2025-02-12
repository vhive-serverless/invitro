package deployment

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

type dirigentDeployer struct{}

type dirigentDeploymentConfiguration struct {
	RegistrationServer string
}

func newDirigentDeployer() *dirigentDeployer {
	return &dirigentDeployer{}
}

func newDirigentDeployerConfiguration(cfg *config.Configuration) dirigentDeploymentConfiguration {
	return dirigentDeploymentConfiguration{
		RegistrationServer: cfg.LoaderConfiguration.DirigentControlPlaneIP,
	}
}

func (*dirigentDeployer) Deploy(cfg *config.Configuration) {
	dirigentConfig := newDirigentDeployerConfiguration(cfg)

	wg := &sync.WaitGroup{}
	wg.Add(len(cfg.Functions))

	for i := 0; i < len(cfg.Functions); i++ {
		go func(idx int) {
			defer wg.Done()

			deployDirigent(
				cfg.Functions[idx],
				dirigentConfig.RegistrationServer,
				cfg.LoaderConfiguration.BusyLoopOnSandboxStartup,
				cfg.LoaderConfiguration.PrepullMode,
				cfg.LoaderConfiguration.RpsRequestedGpu,
			)
		}(i)
	}

	wg.Wait()
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

func deployDirigent(function *common.Function, controlPlaneAddress string, busyLoopOnColdStart bool, prepullMode string, requestedGpu int) {
	metadata := function.DirigentMetadata

	if metadata == nil {
		log.Fatalf("No Dirigent metadata for function %s", function.Name)
	}

	payload := url.Values{
		"name":                {function.Name},
		"image":               {metadata.Image},
		"port_forwarding":     {strconv.Itoa(metadata.Port), metadata.Protocol},
		"scaling_upper_bound": {strconv.Itoa(metadata.ScalingUpperBound)},
		"scaling_lower_bound": {strconv.Itoa(metadata.ScalingLowerBound)},
		"requested_cpu":       {strconv.Itoa(function.CPURequestsMilli)},
		"requested_memory":    {strconv.Itoa(function.MemoryRequestsMiB)},
		"requested_gpu":       {strconv.Itoa(requestedGpu)},
		"env_vars":            metadata.EnvVars,     // FORMAT: arg1=value1 arg2=value2 ...
		"program_args":        metadata.ProgramArgs, // FORMAT: arg1 arg2 ...
		"prepull_mode":        {prepullMode},
	}

	if busyLoopOnColdStart {
		payload["iteration_multiplier"] = []string{strconv.Itoa(function.DirigentMetadata.IterationMultiplier)}
		payload["cold_start_busy_loop_ms"] = []string{strconv.Itoa(function.ColdStartBusyLoopMs)}
	}

	log.Debug(payload)

	resp, err := registrationClient.PostForm(fmt.Sprintf("http://%s/registerService", controlPlaneAddress), payload)
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

	log.Debugf("Got the following endpoints: %v", endpoints)
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
