package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	mc "github.com/vhive-serverless/loader/pkg/metric"
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

type httpInvoker struct {
	client      *http.Client
	loaderCfg   *config.LoaderConfiguration
	dirigentCfg *config.DirigentConfig

	isKnative   bool
	isDandelion bool
	isWorkflow  bool
}

func newHTTPInvoker(cfg *config.Configuration) *httpInvoker {
	lcfg := cfg.LoaderConfiguration
	dcfg := cfg.DirigentConfiguration

	return &httpInvoker{
		client:      CreateHTTPClient(lcfg.GRPCFunctionTimeoutSeconds, lcfg.InvokeProtocol),
		loaderCfg:   lcfg,
		dirigentCfg: dcfg,

		isKnative:   strings.Contains(strings.ToLower(lcfg.Platform), "knative"),
		isDandelion: strings.Contains(strings.ToLower(dcfg.Backend), "dandelion"),
		isWorkflow:  dcfg.Workflow,
	}
}

func (i *httpInvoker) functionInvocationRequest(function *common.Function, runtimeSpec *common.RuntimeSpecification) *http.Request {
	requestBody := &bytes.Buffer{}
	if body := composeBusyLoopBody(function.Name, function.DirigentMetadata.Image, runtimeSpec.Runtime, function.DirigentMetadata.IterationMultiplier); i.isDandelion && body != nil {
		requestBody = body
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s", function.Endpoint), requestBody)
	if err != nil {
		log.Errorf("Failed to create a HTTP request - %v\n", err)
		return nil
	}

	// add system specific stuff
	if !i.isKnative {
		req.Host = function.Name
	}

	req.Header.Set("workload", function.DirigentMetadata.Image)
	req.Header.Set("function", function.Name)
	req.Header.Set("requested_cpu", strconv.Itoa(runtimeSpec.Runtime))
	req.Header.Set("requested_memory", strconv.Itoa(runtimeSpec.Memory))
	req.Header.Set("multiplier", strconv.Itoa(function.DirigentMetadata.IterationMultiplier))
	req.Header.Set("io_percentage", strconv.Itoa(function.DirigentMetadata.IOPercentage))

	if i.isDandelion {
		req.URL.Path = "/hot/matmul"
	}

	return req
}

func (i *httpInvoker) workflowInvocationRequest(wf *common.Function) *http.Request {
	if wf.WorkflowMetadata == nil {
		log.Fatal("Failed to create workflow invocation request: workflow metadata is nil")
	}

	// create request
	reqBody := bytes.NewBufferString(wf.WorkflowMetadata.InvocationRequest)
	req, err := http.NewRequest("POST", "http://"+wf.Endpoint+"/workflow", reqBody)
	if err != nil {
		log.Errorf("Failed to create a HTTP request - %v\n", err)
		return nil
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Host = wf.Name // dirigent takes request name from this

	return req
}

func (i *httpInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}
	start := time.Now()
	record.StartTime = start.UnixMicro()
	record.Instance = function.Name // may get overwritten

	// create request
	var req *http.Request
	if !i.isWorkflow {
		req = i.functionInvocationRequest(function, runtimeSpec)
	} else {
		if !i.isDandelion {
			log.Fatalf("Dirigent workflows are only supported for Dandelion so far!")
		}
		req = i.workflowInvocationRequest(function)
	}
	if req == nil {
		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true
		return false, record
	}

	// send request
	resp, err := i.client.Do(req)
	if err != nil {
		log.Errorf("%s - Failed to send an HTTP request to the server - %v\n", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	record.GRPCConnectionEstablishTime = time.Since(start).Microseconds()

	defer HandleBodyClosing(resp)
	body, err := io.ReadAll(resp.Body)

	if err != nil || resp.StatusCode != http.StatusOK || len(body) == 0 {
		if err != nil {
			log.Errorf("HTTP request failed - %s - %v", function.Name, err)
		} else if len(body) == 0 {
			log.Errorf("HTTP request failed - %s - %s - empty response (status code: %d)", function.Name, function.Endpoint, resp.StatusCode)
		} else if resp.StatusCode != http.StatusOK {
			log.Errorf("HTTP request failed - %s - %s - non-empty response: %v - status code: %d", function.Name, function.Endpoint, string(body), resp.StatusCode)
		}

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record
	}

	if i.isDandelion {
		err = DeserializeDandelionResponse(function, body, record, i.isWorkflow)
		if err != nil {
			log.Warnf("Failed to deserialize Dandelion response - %v - %v", string(body), err)
		}
	} else if i.dirigentCfg.AsyncMode {
		record.AsyncResponseID = string(body)
	} else {
		err = DeserializeDirigentResponse(body, record)
		if err != nil {
			log.Warnf("Failed to deserialize Dirigent response - %v - %v", string(body), err)
		}
	}

	record.ResponseTime = time.Since(start).Microseconds()

	if strings.HasPrefix(string(body), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = 0
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, string(body), float64(0)/1e3, common.Kib2Mib(0))
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}

func DeserializeDirigentResponse(body []byte, record *mc.ExecutionRecord) error {
	var deserializedResponse FunctionResponse
	err := json.Unmarshal(body, &deserializedResponse)
	if err != nil {
		return err
	}

	record.Instance = deserializedResponse.Function
	record.ActualDuration = uint32(deserializedResponse.ExecutionTime)

	return nil
}

func HandleBodyClosing(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}

	err := response.Body.Close()
	if err != nil {
		log.Errorf("Error closing response body - %v", err)
	}
}
