package deployment

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type DirigentDeployer struct {
	FunctionDeployer
}

type DirigentDeploymentConfiguration struct {
	RegistrationServer string
}

func (*DirigentDeployer) Deploy(functions []*common.Function, configuration interface{}) {
	dirigentConfig := configuration.(DirigentDeploymentConfiguration)

	for i := 0; i < len(functions); i++ {
		deployDirigent(functions[i], dirigentConfig.RegistrationServer)
	}
}

func (*DirigentDeployer) Clean() {}

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

func deployDirigent(function *common.Function, controlPlaneAddress string) {
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
