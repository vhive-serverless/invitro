package clients

import (
	"encoding/json"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"io"
	"sync"
	"net/http"
	"crypto/tls"
	"bytes"
	"time"
	"encoding/base64"
)

type awsLambdaInvoker struct {
	announceDoneExe *sync.WaitGroup
}

func newAWSLambdaInvoker(announceDoneExe *sync.WaitGroup) *awsLambdaInvoker {
	return &awsLambdaInvoker{
		announceDoneExe: announceDoneExe,
	}
}

func (i *awsLambdaInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	dataString := fmt.Sprintf(`{"RuntimeInMilliSec": %d, "MemoryInMebiBytes": %d}`, runtimeSpec.Runtime, runtimeSpec.Memory)

	success, executionRecordBase, res := awsHttpInvocation(dataString, function, i.announceDoneExe, false)
	defer res.Body.Close()

	executionRecordBase.RequestedDuration = uint32(runtimeSpec.Runtime * 1e3)
	record := &mc.ExecutionRecord{ExecutionRecordBase: *executionRecordBase}

	if !success {
		return false, record
	}

	// Read the response body
	responseBody, err := io.ReadAll(res.Body)
	if err != nil {
		log.Debugf("Error reading response body:%s", err)
		return false, record
	}

	// Create a variable to store the JSON data
	var httpResBody HTTPResBody

	// Unmarshal the response body into the JSON object
	if err := json.Unmarshal(responseBody, &httpResBody); err != nil {
		log.Debugf("Error unmarshaling JSON:%s", err)
		return false, record
	}

	record.ActualDuration = httpResBody.DurationInMicroSec
	record.ActualMemoryUsage = common.Kib2Mib(httpResBody.MemoryUsageInKb)

	logInvocationSummary(function, &record.ExecutionRecordBase, res)

	return true, record
}

func awsHttpInvocation(dataString string, function *common.Function, AnnounceDoneExe *sync.WaitGroup, tlsSkipVerify bool) (bool, *mc.ExecutionRecordBase, *http.Response) {
	record := &mc.ExecutionRecordBase{}
	start := time.Now()
	record.StartTime = start.UnixMicro()
	record.Instance = function.Name
	requestURL := function.Endpoint

	if tlsSkipVerify {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	var bodyReader io.Reader
	if dataString != "" {
		bodyReader = bytes.NewBuffer([]byte(dataString))
	} else {
		bodyReader = http.NoBody
	}

	req, err := http.NewRequest(http.MethodPost, requestURL, bodyReader)
	if err != nil {
		log.Warnf("http request creation failed for function %s - %s", function.Name, err)
		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true
		return false, record, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("http request for function %s failed - %s", function.Name, err)
		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true
		return false, record, resp
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Debugf("http request for function %s failed - error code: %s", function.Name, resp.Status)
		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true
		return false, record, resp
	}

bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Warnf("Failed to read output %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp
	}

	rawJson, err := base64.StdEncoding.DecodeString(string(bodyBytes))
	if err != nil {
		log.Warnf("Failed to decode base64 output %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp
	}

	var deserializedResponse FunctionResponse
	err = json.Unmarshal(rawJson, &deserializedResponse)
	if err != nil {
		log.Warnf("Failed to deserialize response %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp
	}

	record.Instance = deserializedResponse.Function
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = uint32(deserializedResponse.ExecutionTime)

	return true, record, resp
}

