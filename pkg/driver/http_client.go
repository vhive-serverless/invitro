package driver

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	mc "github.com/eth-easl/loader/pkg/metric"
	log "github.com/sirupsen/logrus"
)

const serverPort = 31001
const recordInstance = "hello"

func InvokeOpenWhisk(function *common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration) (bool, *mc.ActivationRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ActivationRecord{
		RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	requestURL := fmt.Sprintf("https://128.110.218.126:%d/api/v1/web/guest/demo/hello", serverPort)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		log.Debugf("http request creation failed for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.RequestCreationFailed = true

		return false, record
	}

	record.RequestCreationFailed = false

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("http timeout exceeded for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	record.ConnectionTimeout = false
	record.HttpStatusCode = res.StatusCode
	if record.HttpStatusCode < 200 || record.HttpStatusCode >= 300 {
		log.Debugf("http request for function %s failed - error code: %d", function.Name, record.HttpStatusCode)

		return false, record
	}

	record.ActivationID = res.Header.Get("X-Openwhisk-Activation-Id")
	record.Instance = recordInstance
	record.ResponseTime = time.Since(start).Microseconds()

	//read data from OpenWhisk based on the activation ID
	cmd := exec.Command("wsk", "-i", "activation", "get", record.ActivationID)
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		log.Debugf("error reading activation information from OpenWhisk %s - %s", function.Name, err)

		return false, record
	}

	var jsonMap map[string]interface{}
	output := out.String()
	ind := strings.Index(output, "{")
	json.Unmarshal([]byte(output[ind:]), &jsonMap)

	record.ActualDuration = uint32(jsonMap["duration"].(float64)) * 1000 //ms to micro sec
	record.StartType = "hot"
	record.InitTime = 0
	annotations := jsonMap["annotations"].([]interface{})
	for i := 0; i < len(annotations); i++ {
		annotation := annotations[i].(map[string]interface{})

		if annotation["key"] == "waitTime" {
			record.WaitTime = int64(annotation["value"].(float64))
		}

		if annotation["key"] == "initTime" {
			record.StartType = "cold/warm"
			record.InitTime = int64(annotation["value"].(float64)) * 1000 //ms to micro sec
		}
	}

	log.Tracef("(Replied)\t %s: %d[ms]", function.Name, record.ActualDuration)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	fmt.Printf("(Replied)\t %s: %d[ms]", function.Name, record.ActualDuration)
	fmt.Printf("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	fmt.Printf("client: status code: %d\n", res.StatusCode)

	return true, record
}
