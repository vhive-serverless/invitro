package function

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"

	util "github.com/eth-easl/loader/pkg"
	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/eth-easl/loader/server"
)

const (
	port = ":80"
	// See: https://aws.amazon.com/premiumsupport/knowledge-center/lambda-function-retry-timeout-sdk/
	// connectionTimeout = 1 * time.Minute
	// executionTimeout  = 15 * time.Minute
	//! Disable timeout for benchmarking all queuing effects.
	connectionTimeout = 10 * time.Hour
	executionTimeout  = 15 * time.Hour
)

func Invoke(function tc.Function, runtimeRequested int, memoryRequested int) (bool, mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeRequested, memoryRequested)

	var record mc.ExecutionRecord
	record.FuncName = function.Name

	registry.Register(memoryRequested)

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	// conn, err := pools.GetConn(function.Endpoint)
	// if err != nil {
	// 	//! Failures will also be recorded as 0's.
	// 	log.Warnf("gRPC connection failed: %v", err)
	// 	record.Timeout = true
	// 	registry.Deregister(memoryRequested)
	// 	return false, record
	// }
	conn := pools.conns[function.Endpoint]
	grpcClient := server.NewExecutorClient(conn)

	// Contact the server and print out its response.
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), executionTimeout)
	defer cancelExecution()

	response, err := grpcClient.Execute(executionCxt, &server.FaasRequest{
		Message:           "nothing",
		RuntimeInMilliSec: uint32(runtimeRequested),
		MemoryInMebiBytes: uint32(memoryRequested),
	})
	if err != nil {
		log.Warnf("Error in gRPC execution (%s): %v", function.Name, err)
		record.Failed = true
		registry.Deregister(memoryRequested)
		return false, record
	}

	responseTime := time.Since(start).Microseconds()
	record.ResponseTime = responseTime
	record.Load = float64(registry.GetTotalMemoryLoad())
	registry.Deregister(memoryRequested)

	memoryUsage := response.MemoryUsageInKb
	runtime := response.DurationInMicroSec

	record.Memory = memoryUsage
	record.Runtime = runtime

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
		float64(runtime)/1e3, util.Kib2Mib(memoryUsage))
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(responseTime)/1e3)

	return true, record
}
