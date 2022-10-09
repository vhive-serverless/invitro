package driver

import (
	"context"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/workload/proto"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	mc "github.com/eth-easl/loader/pkg/metric"
)

const (
	// TODO: make the following two parameters configurable from outside
	grpcConnectionTimeout = 5 * time.Second
	// Function can execute for at most 15 minutes as in AWS Lambda
	// https://aws.amazon.com/about-aws/whats-new/2018/10/aws-lambda-supports-functions-that-can-run-up-to-15-minutes/
	functionTimeout = 15 * time.Minute
)

func Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, withTracing bool) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		FunctionName:      function.Name,
		RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	dialContext, cancelDialing := context.WithTimeout(context.Background(), grpcConnectionTimeout)
	defer cancelDialing()

	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	dialOptions = append(dialOptions, grpc.WithBlock())
	if withTracing {
		// NOTE: if enabled it will exclude Istio span from the Zipkin trace
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}

	conn, err := grpc.DialContext(dialContext, function.Endpoint, dialOptions...)
	defer gRPCConnectionClose(conn)
	if err != nil {
		log.Warnf("Failed to establish a gRPC connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}

	grpcClient := proto.NewExecutorClient(conn)

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), functionTimeout)
	defer cancelExecution()

	response, err := grpcClient.Execute(executionCxt, &proto.FaasRequest{
		Message:           "nothing",
		RuntimeInMilliSec: uint32(runtimeSpec.Runtime),
		MemoryInMebiBytes: uint32(runtimeSpec.Memory),
	})

	if err != nil {
		log.Warnf("gRPC timeout exceeded for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record
	}

	record.FunctionName = extractInstanceName(response.GetMessage())
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = response.DurationInMicroSec
	record.ActualMemoryUsage = common.Kib2Mib(response.MemoryUsageInKb)

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
		float64(response.DurationInMicroSec)/1e3, common.Kib2Mib(response.MemoryUsageInKb))
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, "-")

	return data[indexOfHyphen+2:]
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		log.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}
