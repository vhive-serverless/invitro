package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type azureFunctionsInvoker struct {
	announceDoneExe *sync.WaitGroup
}

func newAzureFunctionsInvoker(announceDoneExe *sync.WaitGroup) *azureFunctionsInvoker {
	return &azureFunctionsInvoker{
		announceDoneExe: announceDoneExe,
	}
}

func (i *azureFunctionsInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	dataString := fmt.Sprintf(`{"RuntimeInMilliSec": %d, "MemoryInMebiBytes": %d}`, runtimeSpec.Runtime, runtimeSpec.Memory)
	success, executionRecordBase, res, bodyBytes := azureHttpInvocation(dataString, function)

	executionRecordBase.RequestedDuration = uint32(runtimeSpec.Runtime * 1e3)
	record := &mc.ExecutionRecord{ExecutionRecordBase: *executionRecordBase}

	if !success {
		return false, record
	}

	// Create a variable to store the JSON data
	var httpResBody HTTPResBody
	// Unmarshal the response body into the JSON object
	if err := json.Unmarshal(bodyBytes, &httpResBody); err != nil {
		log.Errorf("Error unmarshaling JSON:%s", err)
		return false, record
	}

	record.ActualDuration = httpResBody.DurationInMicroSec
	record.ActualMemoryUsage = common.Kib2Mib(httpResBody.MemoryUsageInKb)

	logInvocationSummary(function, &record.ExecutionRecordBase, res)

	return true, record
}

func azureHttpInvocation(dataString string, function *common.Function) (bool, *mc.ExecutionRecordBase, *http.Response, []byte) {
	record := &mc.ExecutionRecordBase{}

	start := time.Now()
	record.StartTime = start.UnixMicro()
	record.Instance = function.Name
	requestURL := function.Endpoint

	// Prepare request body for POST
	reqBody := bytes.NewBuffer([]byte(dataString))

	// Use POST method with JSON payload as body
	req, err := http.NewRequest(http.MethodPost, requestURL, reqBody)
	if err != nil {
		log.Errorf("http request creation failed for function %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record, nil, nil
	}

	req.Header.Set("Content-Type", "application/json") // JSON payload for POST

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Errorf("http request for function %s failed - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record, resp, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Errorf("Received non-2xx status code for function %s - error code: %s", function.Name, resp.Status)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record, resp, nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("Failed to read response body for function %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp, nil
	}

	var deserializedResponse FunctionResponse
	err = json.Unmarshal(bodyBytes, &deserializedResponse)
	if err != nil {
		log.Errorf("Failed to deserialize response %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp, nil
	}

	record.Instance = deserializedResponse.Function
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = uint32(deserializedResponse.ExecutionTime)

	return true, record, resp, bodyBytes
}
