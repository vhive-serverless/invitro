package invokefunc

import (
	"context"
	"strings"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	"github.com/eth-easl/loader/pkg/workload/promptproto"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	mc "github.com/eth-easl/loader/pkg/metric"
)

func PromptBankInvoke(functions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) ([]float32, float32) {
	initPromptTensor := make([]float32, 128*common.EmbedingDim)
	for i := range initPromptTensor {
		initPromptTensor[i] = 0
	}
	record := &mc.ExecutionRecord{
		RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	dialContext, cancelDialing := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCConnectionTimeoutSeconds)*time.Second)
	defer cancelDialing()

	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	dialOptions = append(dialOptions, grpc.WithBlock())
	if cfg.EnableZipkinTracing {
		// NOTE: if enabled it will exclude Istio span from the Zipkin trace
		dialOptions = append(dialOptions, grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
	}

	conn_list := make([]*grpc.ClientConn, len(functions))
	gpu_list := make([]int, len(functions))
	for function_idx, function := range functions {
		// gpu_list[function_idx], _ = strconv.Atoi(strings.Split(function.Name, "-gpu-")[1])
		gpu_list[function_idx] = 1
		conn, err := grpc.DialContext(dialContext, function.Endpoint, dialOptions...)
		if err != nil {
			log.Debugf("Failed to establish a gRPC connection - %v\n", err)
			record.ResponseTime = time.Since(start).Microseconds()
			record.ConnectionTimeout = true
			return initPromptTensor, 1.0
		}
		conn_list[function_idx] = conn
		// fmt.Printf("gpu is %d, funcname %s\n", gpu_list[function_idx], function.Name)
	}

	for i := 0; i < len(functions); i++ {
		defer gRPCConnectionClose(conn_list[i])
	}

	// executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	leaseTime := 30
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)

	promptTensor := make([]float32, 128*common.EmbedingDim)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	// log.Debugf("SingleInvoke gRPC step 1")
	if !strings.Contains(functions[0].Name, "promptbank-") {
		return initPromptTensor, 1.0
	}

	responses := make([]promptproto.PromptReply, 32)

	// create grpc clients
	grpcClients := make([]promptproto.ExecutorClient, len(functions))
	for conn_idx, conn := range conn_list {
		grpcClients[conn_idx] = promptproto.NewExecutorClient(conn)
	}

	// ActualDuration := uint32(0)

	iteration_per_call := 100
	send_messages := "Can you condense the sentence into a shorter version without losing its meaning? @"
	for i := 0; i < iteration_per_call; i++ {
		for bsz := 0; bsz < common.BszPerDevice; bsz++ {
			send_messages = send_messages + "; Can you condense the sentence into a shorter version without losing its meaning? @"
		}
	}
	deploymentID := 0

onemore:
	response, err := grpcClients[deploymentID].Execute(executionCxt, &promptproto.PromptRequest{
		Message: send_messages,
	})
	if err != nil {
		cancelExecution()
		executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
		goto onemore
	}

	responses[0] = *response
	record.ActualDuration += responses[0].DurationInMicroSec

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Microseconds()

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", functions[0].Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", functions[0].Name, float64(record.ResponseTime)/1e3)
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptTensor), sum(responses[0].PromptTensor))
	cancelExecution()
	return response.PromptTensor, 0.5
}
