package function

import (
	"context"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"

	mc "github.com/eth-easl/loader/pkg/metric"
	tc "github.com/eth-easl/loader/pkg/trace"
	rpc "github.com/eth-easl/loader/server"
)

func Invoke(ctx context.Context, function tc.Function, gen tc.FunctionSpecsGen) (bool, mc.ExecutionRecord) {
	//! Failed execution will also be recorded w/ 0's.

	runtimeRequested, memoryRequested := gen(function)

	log.Infof("(Invoke)\t %s: %d[µs], %d[MiB]", function.Name, runtimeRequested*int(math.Pow10(3)), memoryRequested)

	var record mc.ExecutionRecord
	record.FuncName = function.Name

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	conn, err := grpc.DialContext(ctx, function.Endpoint, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Warnf("Failed to connect: %v", err)
		record.Timeout = true
		return false, record
	}
	defer conn.Close()

	//TODO: Write a function stub based upon the Producer of vSwarm.
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
		log.Warnf("%s: err=%v", function.Name, err)
		record.Failed = true
		return false, record
	}
	// log.Info("gRPC response: ", reply.Response)
	memoryUsage := response.MemoryUsageInKb
	runtime := response.DurationInMicroSec

	record.Memory = memoryUsage
	record.Runtime = runtime

	log.Infof("(gRPC)\t %s: %d[µs], %d[KB]", function.Name, runtime, memoryUsage)

	responseTime := time.Since(start).Microseconds()
	record.ResponseTime = responseTime
	log.Infof("(Latency)\t %s: %d[µs]\n", function.Name, responseTime)

	return true, record
}
