package driver

import (
	"bytes"
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"go.mongodb.org/mongo-driver/bson"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type FunctionResponse struct {
	Status        string `json:"Status"`
	Function      string `json:"Function"`
	MachineName   string `json:"MachineName"`
	ExecutionTime int64  `json:"ExecutionTime"`
}

type InputItem struct {
	Identifier string `bson:"identifier"`
	Key        int64  `bson:"key"`
	Data       []byte `bson:"data"`
}

type InputSet struct {
	Identifier string      `bson:"identifier"`
	Items      []InputItem `bson:"items"`
}

type MatrixRequest struct {
	Name string     `bson:"name"`
	Sets []InputSet `bson:"sets"`
}

func InvokeDirigent(function *common.Function, runtimeSpec *common.RuntimeSpecification, client *http.Client, isDandelionOptional ...bool) (bool, *mc.ExecutionRecord) {
	isDandelion := false
	if len(isDandelionOptional) > 0 {
		isDandelion = true
	}
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	var requestBody *bytes.Buffer
	if isDandelion {
		matRequest := MatrixRequest{
			Name: function.Name,
			Sets: []InputSet{
				{
					Identifier: "",
					Items: []InputItem{
						{
							Identifier: "",
							Key:        0,
							Data:       []byte{1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0},
						},
					},
				},
			},
		}
		matRequestBody, err := bson.Marshal(matRequest)
		if err != nil {
			log.Debugf("Error encoding dandelion invoke request:", err)
			return false, record
		}
		requestBody = bytes.NewBuffer(matRequestBody)
	} else {
		requestBody = &bytes.Buffer{}
	}

	req, err := http.NewRequest("GET", "http://"+function.Endpoint, requestBody)

	if err != nil {
		log.Errorf("Failed to create a HTTP request - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	req.Host = function.Name

	req.Header.Set("workload", function.DirigentMetadata.Image)
	req.Header.Set("function", function.Name)
	req.Header.Set("requested_cpu", strconv.Itoa(runtimeSpec.Runtime))
	req.Header.Set("requested_memory", strconv.Itoa(runtimeSpec.Memory))
	req.Header.Set("multiplier", strconv.Itoa(function.DirigentMetadata.IterationMultiplier))

	if isDandelion {
		req.URL.Path = "/hot/matmul"
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("%s - Failed to send an HTTP request to the server - %v\n", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	record.GRPCConnectionEstablishTime = time.Since(start).Microseconds()

	body, err := io.ReadAll(resp.Body)
	defer handleBodyClosing(resp)
	log.Debugf("received invocation resp for function %s, body length=%v", function.Name, len(body))

	if err != nil || resp == nil || resp.StatusCode != http.StatusOK || len(body) == 0 {
		if err != nil {
			log.Errorf("HTTP request failed - %s - %v", function.Name, err)
		} else if resp == nil || len(body) == 0 {
			log.Errorf("HTTP request failed - %s - %s - empty response (status code: %d)", function.Name, function.Endpoint, resp.StatusCode)
		} else if resp.StatusCode != http.StatusOK {
			log.Errorf("HTTP request failed - %s - %s - non-empty response: %v - status code: %d", function.Name, function.Endpoint, string(body), resp.StatusCode)
		}

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record
	}

	var deserializedResponse FunctionResponse
	err = json.Unmarshal(body, &deserializedResponse)
	if err != nil {
		log.Warnf("Failed to deserialize Dirigent response.")
	}

	record.Instance = deserializedResponse.Function
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = uint32(deserializedResponse.ExecutionTime)

	if strings.HasPrefix(string(body), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = 0 //
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, string(body), float64(0)/1e3, common.Kib2Mib(0))
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}

func handleBodyClosing(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}

	err := response.Body.Close()
	if err != nil {
		log.Errorf("Error closing response body - %v", err)
	}
}
