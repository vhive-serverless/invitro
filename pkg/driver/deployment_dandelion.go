package driver

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type RegisterFunction struct {
	Name        string  `bson:"name"`
	ContextSize uint64  `bson:"context_size"`
	EngineType  string  `bson:"engine_type"`
	Binary      []int32 `bson:"binary"`
}

func DeployFunctionsDandelion(controlPlaneAddress string, functions []*common.Function) {
	for i := 0; i < len(functions); i++ {
		function := functions[i]
		name := functions[i].Name

		endpoint := fmt.Sprintf("http://%s/", controlPlaneAddress)
		metadata := function.DirigentMetadata
		payload := url.Values{
			"name":                {name},
			"image":               {metadata.Image},
			"port_forwarding":     {strconv.Itoa(metadata.Port), metadata.Protocol},
			"scaling_upper_bound": {strconv.Itoa(metadata.ScalingUpperBound)},
			"scaling_lower_bound": {strconv.Itoa(metadata.ScalingLowerBound)},
			"requested_cpu":       {strconv.Itoa(function.CPURequestsMilli)},
			"requested_memory":    {strconv.Itoa(function.MemoryRequestsMiB)},
			"dandelion_request":   {strconv.FormatBool(true)},
		}
		resp, err := http.PostForm(endpoint, payload)
		if err != nil {
			logrus.Errorf("failed to reigster function", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			logrus.Debugf("register function %s ok!", name)
		} else {
			logrus.Debugf("register function %s failed, error code = %v", name, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			logrus.Error("Failed to read response body.")
			return
		}

		logrus.Debugf("registtter function %s", function.Name)

		endpoints := strings.Split(string(body), ";")
		if len(endpoints) == 0 {
			logrus.Error("Function registration returned no data plane(s).")
			return
		}
		function.Endpoint = endpoints[rand.Intn(len(endpoints))]
	}
}
