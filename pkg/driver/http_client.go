package driver

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	mc "github.com/eth-easl/loader/pkg/metric"
	log "github.com/sirupsen/logrus"
)

type ActivationMetadata struct {
	Duration  uint32 //ms
	StartType string //"hot" or "cold"
	WaitTime  int64  //micro seconds
	InitTime  int64  //ms
}

func InvokeOpenWhisk(function *common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, AnnouceDoneExe *sync.WaitGroup) (bool, *mc.ExecutionRecordOpenWhisk) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecordOpenWhisk{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	requestURL := function.Endpoint
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		log.Debugf("http request creation failed for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		AnnouceDoneExe.Done()

		return false, record
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("http timeout exceeded for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		AnnouceDoneExe.Done()

		return false, record
	}

	record.HttpStatusCode = res.StatusCode
	if record.HttpStatusCode < 200 || record.HttpStatusCode >= 300 {
		log.Debugf("http request for function %s failed - error code: %d", function.Name, record.HttpStatusCode)

		AnnouceDoneExe.Done()

		return false, record
	}

	record.ActivationID = res.Header.Get("X-Openwhisk-Activation-Id")
	record.Instance = function.Name
	record.ResponseTime = time.Since(start).Microseconds()

	AnnouceDoneExe.Done()
	AnnouceDoneExe.Wait()

	//read data from OpenWhisk based on the activation ID
	cmd := exec.Command("wsk", "-i", "activation", "get", record.ActivationID)
	time.Sleep(2 * time.Second)
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Debugf("error reading activation information from OpenWhisk %s - %s", function.Name, err)

		return false, record
	}

	err, activationMetadata := parseActivationMetadata(out.String())
	if err != nil {
		log.Debugf("error parsing activation metadata %s - %s", function.Name, err)

		return false, record
	}

	record.ActualDuration = activationMetadata.Duration * 1000 //ms to micro sec
	record.StartType = activationMetadata.StartType
	record.InitTime = activationMetadata.InitTime * 1000 //ms to micro sec
	record.WaitTime = activationMetadata.WaitTime

	log.Tracef("(Replied)\t %s: %d[ms]", function.Name, record.ActualDuration)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	log.Tracef("(Client status code) %s: %d", function.Name, record.HttpStatusCode)

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
	result.StartType = "hot"
	result.InitTime = 0
	annotations := jsonMap["annotations"].([]interface{})
	for i := 0; i < len(annotations); i++ {
		annotation := annotations[i].(map[string]interface{})

		if annotation["key"] == "waitTime" {
			result.WaitTime = int64(annotation["value"].(float64))
		} else if annotation["key"] == "initTime" {
			result.StartType = "cold"
			result.InitTime = int64(annotation["value"].(float64))
		}
	}

	return nil, result
}
