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

func Invoke(ctx context.Context, function tc.Function, gen tc.FunctionSpecsGen) (bool, mc.LatencyRecord) {
	runtimeRequested, memoryRequested := gen(function)

	log.Infof("(Invoke)\t %s: %d[µs], %d[MiB]", function.GetName(), runtimeRequested*int(math.Pow10(3)), memoryRequested)

	var record mc.LatencyRecord
	record.FuncName = function.GetName()

	// Start latency measurement.
	start := time.Now()
	record.Timestamp = start.UnixMicro()

	conn, err := grpc.DialContext(ctx, function.GetUrl(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Warnf("Failed to connect: %v", err)
		return false, mc.LatencyRecord{}
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
		log.Warnf("%s: err=%v", function.GetName(), err)
		return false, mc.LatencyRecord{}
	}
	// log.Info("gRPC response: ", reply.Response)
	memoryUsage := response.MemoryUsageInKb
	runtime := response.DurationInMicroSec

	record.Memory = memoryUsage
	record.Runtime = runtime

	log.Infof("(gRPC)\t %s: %d[µs], %d[KB]", function.GetName(), runtime, memoryUsage)

	latency := time.Since(start).Microseconds()
	record.Latency = latency
	log.Infof("(Latency)\t %s: %d[µs]\n", function.GetName(), latency)

	return true, record
}
