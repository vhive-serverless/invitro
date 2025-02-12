package clients

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type FunctionResponse struct {
	Status        string `json:"Status"`
	Function      string `json:"Function"`
	MachineName   string `json:"MachineName"`
	ExecutionTime int64  `json:"ExecutionTime"`
}

type httpInvoker struct {
	client *http.Client
	cfg    *config.LoaderConfiguration
}

func newHTTPInvoker(cfg *config.LoaderConfiguration) *httpInvoker {
	return &httpInvoker{
		client: CreateHTTPClient(cfg.GRPCFunctionTimeoutSeconds, cfg.InvokeProtocol),
		cfg:    cfg,
	}
}

var payload []byte = nil
var contentType string = "application/octet-stream"

func CreateRandomPayload(sizeInMB float64) *bytes.Buffer {
	if payload == nil {
		byteCount := int(sizeInMB * 1024.0 * 1024.0) // MB -> B
		payload = make([]byte, byteCount)

		n, err := rand.Read(payload)
		if err != nil || n != byteCount {
			log.Errorf("Failed to generate random %d bytes.", byteCount)
		}
	}

	return bytes.NewBuffer(payload)
}

func CreateFilePayload(filePath string) *bytes.Buffer {
	if payload == nil {
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatalf("Failed to open file %s: %v", filePath, err)
		}

		buffer := &bytes.Buffer{}
		writer := multipart.NewWriter(buffer)
		part, err := writer.CreateFormFile("images", "invitro.payload")
		if err != nil {
			log.Fatalf("Failed to create form file: %v", err)
		}

		if _, err = io.Copy(part, file); err != nil {
			log.Fatalf("Failed to enter file into the form: %v", err)
		}
		if err = writer.Close(); err != nil {
			log.Fatalf("Failed to close writer: %v", err)
		}

		payload = buffer.Bytes()
		contentType = writer.FormDataContentType()
		return buffer
	}

	return bytes.NewBuffer(payload)
}

func (i *httpInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	isDandelion := strings.Contains(strings.ToLower(i.cfg.Platform), "dandelion")
	isKnative := strings.Contains(strings.ToLower(i.cfg.Platform), "knative")

	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////

	requestBody := &bytes.Buffer{}
	/*if body := composeDandelionMatMulBody(function.Name); isDandelion && body != nil {
		requestBody = body
	}*/
	if body := composeBusyLoopBody(function.Name, function.DirigentMetadata.Image, runtimeSpec.Runtime, function.DirigentMetadata.IterationMultiplier); isDandelion && body != nil {
		requestBody = body
	}
	if i.cfg.RpsTarget != 0 {
		ts := time.Now()
		if i.cfg.RpsFile != "" {
			requestBody = CreateFilePayload(i.cfg.RpsFile)
			log.Debugf("Took %v to create file body.", time.Since(ts))
		} else {
			requestBody = CreateRandomPayload(i.cfg.RpsDataSizeMB)
			log.Debugf("Took %v to generate request body.", time.Since(ts))
		}
	}

	start := time.Now()
	record.StartTime = start.UnixMicro()

	req, err := http.NewRequest("POST", "http://"+function.Endpoint, requestBody)
	req.Header.Add("Content-Type", contentType)
	if err != nil {
		log.Errorf("Failed to create a HTTP request - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	// add system specific stuff
	if !isKnative {
		req.Host = function.Name
	}

	req.Header.Set("workload", function.DirigentMetadata.Image)
	req.Header.Set("function", function.Name)
	req.Header.Set("requested_cpu", strconv.Itoa(runtimeSpec.Runtime))
	req.Header.Set("requested_memory", strconv.Itoa(runtimeSpec.Memory))
	req.Header.Set("multiplier", strconv.Itoa(function.DirigentMetadata.IterationMultiplier))
	req.Header.Set("io_percentage", strconv.Itoa(function.DirigentMetadata.IOPercentage))

	if isDandelion {
		req.URL.Path = "/hot/matmul"
	}

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

	if isDandelion {
		err = DeserializeDandelionResponse(function, body, record)
		if err != nil {
			log.Warnf("Failed to deserialize Dandelion response - %v - %v", string(body), err)
		}
	} else if i.cfg.AsyncMode {
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
