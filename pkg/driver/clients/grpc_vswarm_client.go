package clients

import (
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/common"
	proto "github.com/vhive-serverless/vSwarm/utils/protobuf/helloworld"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"github.com/sirupsen/logrus"
	"strings"
	"time"
	"github.com/google/uuid"
	"context"
)

type grpcVSwarmInvoker struct {
	cfg *config.LoaderConfiguration
}

func newGRPCVSwarmInvoker(cfg *config.LoaderConfiguration) *grpcVSwarmInvoker {
	return &grpcVSwarmInvoker{
		cfg: cfg,
	}
}

func (i *grpcVSwarmInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	logrus.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

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

	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if i.cfg.EnableZipkinTracing {
		dialOptions = append(dialOptions, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	}

	grpcStart := time.Now()

	conn, err := grpc.NewClient(function.Endpoint, dialOptions...)
	if err != nil {
		logrus.Debugf("Failed to establish a gRPC connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}
	defer gRPCConnectionClose(conn)

	record.GRPCConnectionEstablishTime = time.Since(grpcStart).Microseconds()

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(i.cfg.GRPCFunctionTimeoutSeconds)*time.Second)

	defer cancelExecution()

	grpcClient := proto.NewGreeterClient(conn)
	response, err := grpcClient.SayHello(executionCxt, &proto.HelloRequest{
		Name: "Invoke Relay",
		VHiveMetadata: MakeVHiveMetadata(
			uuid.New().String(),
			uuid.New().String(),
			time.Now().UTC(),
		),
	})
	if err != nil {
		logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.FunctionTimeout = true

		return false, record
	}
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = uint32(record.ResponseTime)
	record.Instance = extractInstanceName(response.GetMessage())
	if strings.HasPrefix(response.GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(0)
	}
	
	logrus.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}