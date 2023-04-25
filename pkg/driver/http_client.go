package driver

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"net/http"
	"os/exec"
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

	//read response
	record.ActivationID = res.Header.Get("X-Openwhisk-Activation-Id")
	record.HttpStatusCode = res.StatusCode
	record.Instance = recordInstance
	record.ResponseTime = time.Since(start).Microseconds()

	cmd := exec.Command("wsk", "-i", "activation", "get", record.ActivationID)
	var out bytes.Buffer
	cmd.Stdout = &out

	er := cmd.Run()

	if er != nil {
		log.Fatal(err)
	}

	fmt.Println(out.String())

	// record.ActualDuration = response.DurationInMicroSec
	record.ActualDuration = uint32(record.ResponseTime) //change later

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		log.Debugf("http request for function %s failed - error code: %d", function.Name, res.StatusCode)

		return false, record
	}

	// log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
	// 	float64(response.DurationInMicroSec)/1e3, common.Kib2Mib(response.MemoryUsageInKb))
	// log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	log.Tracef("(Replied)\t %s:", function.Name)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	fmt.Printf("(Replied)\t %s:", function.Name)
	fmt.Printf("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	fmt.Printf("client: status code: %d\n", res.StatusCode)

	return true, record
}
