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

var registry = LoadRegistry{}

var port = ":80"

func Invoke(ctx context.Context, function tc.Function, gen tc.FunctionSpecsGen) (bool, mc.ExecutionRecord) {
	runtimeRequested, memoryRequested := gen(function)
	log.Infof("(Invoke)\t %s: %d[µs], %d[MiB]", function.Name, runtimeRequested*1000, memoryRequested)

	var record mc.ExecutionRecord
	record.FuncName = function.Name

	registry.Register(memoryRequested)

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	conn, err := grpc.DialContext(ctx, function.Endpoint+port, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		//! Failures will also be recorded with 0's.
		log.Warnf("gRPC connection failed: %v", err)
		record.Timeout = true
		registry.Deregister(memoryRequested)
		return false, record
	}
	defer conn.Close()

	grpcClient := rpc.NewExecutorClient(conn)
	// Contact the server and print out its response.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	response, err := grpcClient.Execute(ctx, &rpc.FaasRequest{
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

	// log.Info("gRPC response: ", reply.Response)
	memoryUsage := response.MemoryUsageInKb
	runtime := response.DurationInMicroSec

	record.Memory = memoryUsage
	record.Runtime = runtime

	log.Infof("(Replied)\t %s: %d[µs], %d[KB]", function.Name, runtime, memoryUsage)
	log.Infof("(E2E Latency) %s: %d[µs]\n", function.Name, responseTime)

	return true, record
}
