package driver

import (
	"context"
	"crypto/tls"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"golang.org/x/net/http2"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func InvokeDirigent(function *common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	client := http.Client{
		Timeout: time.Duration(cfg.GRPCConnectionTimeoutSeconds) * time.Second,
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return net.Dial(network, addr)
			},
			DisableCompression: true,
		},
	}

	// TODO: add execution time and memory usage runtime args
	req, err := http.NewRequest("GET", "http://"+function.Endpoint, nil)
	req.Host = function.Name

	resp, err := client.Do(req)
	if err != nil {
		log.Debugf("Failed to establish a HTTP connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	record.GRPCConnectionEstablishTime = time.Since(start).Microseconds()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp == nil || resp.StatusCode != http.StatusOK {
		log.Debugf("HTTP timeout for function %s", function.Name)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record
	}

	record.Instance = extractInstanceName(string(body))
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = 0

	if strings.HasPrefix(string(body), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = 0 //
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, string(body),
		float64(0)/1e3, common.Kib2Mib(0))
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}
