/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package driver

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type ActivationMetadata struct {
	Duration  uint32 //ms
	StartType mc.StartType
	WaitTime  int64 //ms
	InitTime  int64 //ms
}

type HTTPResBody struct {
	DurationInMicroSec uint32 `json:"DurationInMicroSec"`
	MemoryUsageInKb    uint32 `json:"MemoryUsageInKb"`
}

func InvokeOpenWhisk(function *common.Function, runtimeSpec *common.RuntimeSpecification, AnnounceDoneExe *sync.WaitGroup, ReadOpenWhiskMetadata *sync.Mutex) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	qs := fmt.Sprintf("cpu=%d", runtimeSpec.Runtime)

	success, executionRecordBase, res := httpInvocation(qs, function, AnnounceDoneExe, true)
	//AnnounceDoneExe.Wait() // To postpone querying OpenWhisk during the experiment for performance reasons (Issue 329: https://github.com/vhive-serverless/invitro/issues/329)

	executionRecordBase.RequestedDuration = uint32(runtimeSpec.Runtime * 1e3)
	record := &mc.ExecutionRecord{ExecutionRecordBase: *executionRecordBase}

	if !success {
		return false, record
	}

	/*activationID := res.Header.Get("X-Openwhisk-Activation-Id")

	ReadOpenWhiskMetadata.Lock()

	//read data from OpenWhisk based on the activation ID
	cmd := exec.Command("wsk", "-i", "activation", "get", activationID)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Debugf("error reading activation information from OpenWhisk %s - %s", function.Name, err)

		ReadOpenWhiskMetadata.Unlock()

		return false, record
	}

	ReadOpenWhiskMetadata.Unlock()

	err, activationMetadata := parseActivationMetadata(out.String())
	if err != nil {
		log.Debugf("error parsing activation metadata %s - %s", function.Name, err)

		return false, record
	}*/

	//record.ActualDuration = activationMetadata.Duration * 1000 //ms to micro sec
	/*record.StartType = activationMetadata.StartType
	record.InitTime = activationMetadata.InitTime * 1000 //ms to micro sec
	record.WaitTime = activationMetadata.WaitTime * 1000 //ms to micro sec*/

	logInvocationSummary(function, &record.ExecutionRecordBase, res)

	return true, record
}

func parseActivationMetadata(response string) (error, ActivationMetadata) {
	var result ActivationMetadata
	var jsonMap map[string]interface{}

	ind := strings.Index(response, "{")
	err := json.Unmarshal([]byte(response[ind:]), &jsonMap)
	if err != nil {
		return err, result
	}

	result.Duration = uint32(jsonMap["duration"].(float64))
	result.StartType = mc.Hot
	result.InitTime = 0
	annotations := jsonMap["annotations"].([]interface{})
	for i := 0; i < len(annotations); i++ {
		annotation := annotations[i].(map[string]interface{})

		if annotation["key"] == "waitTime" {
			result.WaitTime = int64(annotation["value"].(float64))
		} else if annotation["key"] == "initTime" {
			result.StartType = mc.Cold
			result.InitTime = int64(annotation["value"].(float64))
		}
	}

	return nil, result
}

func InvokeAWSLambda(function *common.Function, runtimeSpec *common.RuntimeSpecification, AnnounceDoneExe *sync.WaitGroup) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	dataString := fmt.Sprintf(`{"RuntimeInMilliSec": %d, "MemoryInMebiBytes": %d}`, runtimeSpec.Runtime, runtimeSpec.Memory)
	success, executionRecordBase, res := httpInvocation(dataString, function, AnnounceDoneExe, false)

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

func httpInvocation(dataString string, function *common.Function, AnnounceDoneExe *sync.WaitGroup, tlsSkipVerify bool) (bool, *mc.ExecutionRecordBase, *http.Response) {
	defer AnnounceDoneExe.Done()

	record := &mc.ExecutionRecordBase{}

	start := time.Now()
	record.StartTime = start.UnixMicro()
	record.Instance = function.Name
	requestURL := function.Endpoint

	if tlsSkipVerify {
		http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	if dataString != "" {
		requestURL += "?" + dataString
	}
	req, err := http.NewRequest(http.MethodGet, requestURL, bytes.NewBuffer([]byte("")))
	req.Header.Set("Content-Type", "application/json") // To avoid data being base64encoded

	if err != nil {
		log.Debugf("http request creation failed for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record, nil
	}

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
		log.Debugf("Failed to read output %s - %v", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record, resp
	}

	rawJson, err := base64.StdEncoding.DecodeString(string(bodyBytes))
	if err != nil {
		log.Debugf("Failed to decode base64 output %s - %v", function.Name, err)

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

func logInvocationSummary(function *common.Function, record *mc.ExecutionRecordBase, res *http.Response) {
	log.Tracef("(Replied)\t %s: %d[ms]", function.Name, record.ActualDuration)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	log.Tracef("(Client status code) %s: %d", function.Name, res.StatusCode)
}
