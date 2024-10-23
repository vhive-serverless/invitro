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
	"strconv"
	"strings"
	"time"
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

	for i := 0; i < len(cfg.Functions); i++ {
		deployDirigent(cfg.Functions[i], dirigentConfig.RegistrationServer, cfg.LoaderConfiguration.BusyLoopOnSandboxStartup)
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

const CSVColumnDelimiter = " "

func prepareEnvVars(data string) []string {
	// FORMAT: arg1=value1 arg2=value2
	return strings.Split(data, CSVColumnDelimiter)
}

func prepareProgramArgs(data string) []string {
	// FORMAT: arg1 val1 arg2 val2
	return strings.Split(data, CSVColumnDelimiter)
}

func deployDirigent(function *common.Function, controlPlaneAddress string, busyLoopOnColdStart bool) {
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
		"env_vars":            prepareEnvVars(metadata.EnvVars),
		"program_args":        prepareProgramArgs(metadata.ProgramArgs),
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

	endpoints := strings.Split(string(body), ";")
	if len(endpoints) == 0 {
		log.Error("Function registration returned no data plane(s).")
		return
	}
	function.Endpoint = endpoints[rand.Intn(len(endpoints))]
}
