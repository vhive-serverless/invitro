package clients

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

type awsLambdaInvoker struct {
	announceDoneExe *sync.WaitGroup
}

func newAWSLambdaInvoker(announceDoneExe *sync.WaitGroup) *awsLambdaInvoker {
	return &awsLambdaInvoker{
		announceDoneExe: announceDoneExe,
	}
}

func (i *awsLambdaInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *common.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	dataString := fmt.Sprintf(`{"RuntimeInMilliSec": %d, "MemoryInMebiBytes": %d}`, runtimeSpec.Runtime, runtimeSpec.Memory)
	success, executionRecordBase, res := httpInvocation(dataString, function, i.announceDoneExe, false)

	executionRecordBase.RequestedDuration = uint32(runtimeSpec.Runtime * 1e3)
	record := &common.ExecutionRecord{ExecutionRecordBase: *executionRecordBase}

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
