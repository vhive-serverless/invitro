package function

import (
	"context"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
	rpc "github.com/eth-easl/loader/server"
)

const (
	port = ":80"
	// See: https://aws.amazon.com/premiumsupport/knowledge-center/lambda-function-retry-timeout-sdk/
	connectionTimeout = 10 * time.Hour
	executionTimeout  = 15 * time.Hour
)

var registry = LoadRegistry{}

func Invoke(function tc.Function, gen tc.FunctionSpecsGen) (bool, mc.ExecutionRecord) {
	runtimeRequested, memoryRequested := gen(function)
	log.Infof("(Invoke)\t %s: %d[µs], %d[MiB]", function.Name, runtimeRequested*1000, memoryRequested)

	var record mc.ExecutionRecord
	record.FuncName = function.Name

	registry.Register(memoryRequested)

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	//* Use the maximum socket timeout from AWS (1min).
	dailCxt, cancelDailing := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancelDailing()

	conn, err := grpc.DialContext(dailCxt, function.Endpoint+port, grpc.WithInsecure(), grpc.WithBlock())
	// defer dclose(conn)
	if err != nil {
		//! Failures will also be recorded with 0's.
		log.Warnf("gRPC connection failed: %v", err)
		record.Timeout = true
		registry.Deregister(memoryRequested)
		return false, record
	}

	grpcClient := rpc.NewExecutorClient(conn)
	// Contact the server and print out its response.
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), executionTimeout)
	defer cancelExecution()

	response, err := grpcClient.Execute(executionCxt, &rpc.FaasRequest{
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

	log.Infof("(Replied)\t %s: %d[µs], %d[KB]", function.Name, runtime, memoryUsage)
	log.Infof("(E2E Latency) %s: %d[µs]\n", function.Name, responseTime)

	return true, record
}

// func dclose(c io.Closer) {
// 	if err := c.Close(); err != nil {
// 		log.Warn("Connection closing error: ", err)
// 	}
// }
