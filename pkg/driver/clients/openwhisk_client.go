package clients

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
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

type openWhiskInvoker struct {
	announceDoneExe       *sync.WaitGroup
	readOpenWhiskMetadata *sync.Mutex
}

func newOpenWhiskInvoker(announceDoneExe *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex) *openWhiskInvoker {
	return &openWhiskInvoker{
		announceDoneExe:       announceDoneExe,
		readOpenWhiskMetadata: readOpenWhiskMetadata,
	}
}

func (i *openWhiskInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	qs := fmt.Sprintf("cpu=%d", runtimeSpec.Runtime)

	success, executionRecordBase, res := httpInvocation(qs, function, i.announceDoneExe, true)
	//announceDoneExe.Wait() // To postpone querying OpenWhisk during the experiment for performance reasons (Issue 329: https://github.com/vhive-serverless/invitro/issues/329)

	executionRecordBase.RequestedDuration = uint32(runtimeSpec.Runtime * 1e3)
	record := &mc.ExecutionRecord{ExecutionRecordBase: *executionRecordBase}
	if !success {
		return false, record
	}

	/*activationID := res.Header.Get("X-Openwhisk-Activation-Id")
	readOpenWhiskMetadata.Lock()
	//read data from OpenWhisk based on the activation ID
	cmd := exec.Command("wsk", "-i", "activation", "get", activationID)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		log.Debugf("error reading activation information from OpenWhisk %s - %s", function.Name, err)
		readOpenWhiskMetadata.Unlock()
		return false, record
	}
	readOpenWhiskMetadata.Unlock()
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

/*func parseActivationMetadata(response string) (error, ActivationMetadata) {
	var result ActivationMetadata
	var jsonMap map[string]interface{}
	@@ -128,46 +141,11 @@ func parseActivationMetadata(response string) (error, ActivationMetadata) {
	}
	return nil, result
}*/

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
	if err != nil {
		log.Warnf("http request creation failed for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record, nil
	}

	req.Header.Set("Content-Type", "application/json") // To avoid data being base64encoded

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

func logInvocationSummary(function *common.Function, record *mc.ExecutionRecordBase, res *http.Response) {
	log.Tracef("(Replied)\t %s: %d[ms]", function.Name, record.ActualDuration)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	log.Tracef("(Client status code) %s: %d", function.Name, res.StatusCode)
}
