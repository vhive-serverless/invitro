package invokefunc

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	"github.com/eth-easl/loader/pkg/workload/proto"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	mc "github.com/eth-easl/loader/pkg/metric"
)

func HiveDInvoke(functions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {

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
		gpu_list[function_idx], _ = strconv.Atoi(strings.Split(function.Name, "-gpu-")[1])
		conn, err := grpc.DialContext(dialContext, function.Endpoint, dialOptions...)
		if err != nil {
			log.Debugf("Failed to establish a gRPC connection - %v\n", err)
			record.ResponseTime = time.Since(start).Microseconds()
			record.ConnectionTimeout = true
			return false, record
		}
		conn_list[function_idx] = conn
		// fmt.Printf("gpu is %d, funcname %s\n", gpu_list[function_idx], function.Name)
	}

	for i := 0; i < len(functions); i++ {
		defer gRPCConnectionClose(conn_list[i])
	}

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	defer cancelExecution()

	promptTensor := make([]float32, 128)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	// log.Debugf("SingleInvoke gRPC step 1")
	if !strings.Contains(functions[0].Name, "gpt") {
		return false, record
	}
	// log.Debugf("SingleInvoke gRPC step 2")

	// minReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice

	responses := make([]proto.FaasReply, 32)

	// create grpc clients
	grpcClients := make([]proto.ExecutorClient, len(functions))
	for conn_idx, conn := range conn_list {
		grpcClients[conn_idx] = proto.NewExecutorClient(conn)
	}

	// ActualDuration := uint32(0)

	iteration_per_call := 100
	send_messages := "Can you condense the sentence into a shorter version without losing its meaning?"
	for i := 0; i < iteration_per_call; i++ {
		for bsz := 0; bsz < common.BszPerDevice; bsz++ {
			send_messages = send_messages + "; Can you condense the sentence into a shorter version without losing its meaning?"
		}
	}
	totalBatchSize := runtimeSpec.Stats.BatchSize
	maxReplicas := totalBatchSize / common.BszPerDevice
	maxDeploymentGPUID := findIndex(common.GPUSet, maxReplicas)
	curDeploymentGPUID := 0
	lastResponseTime := int64(math.MaxInt64)

	for curIter := 0; curIter < runtimeSpec.Stats.Iterations/iteration_per_call; curIter++ {
	onemore:
		curTime := time.Now()
		deploymentID := findIndex(gpu_list, common.GPUSet[curDeploymentGPUID])
		accumulation_steps := totalBatchSize / gpu_list[deploymentID] / common.BszPerDevice

		response, err := grpcClients[deploymentID].Execute(executionCxt, &proto.FaasRequest{
			Message:              send_messages,
			Batchsize:            uint32(common.BszPerDevice),
			RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * iteration_per_call * accumulation_steps),
			GpuMemoryInMebiBytes: 123,
			PromptTensor:         promptTensor,
		})

		if err != nil {
			log.Debugf("gRPC timeout exceeded for function %s - %s", functions[0].Name, err)
			curDeploymentGPUID = curDeploymentGPUID - 1
			if curDeploymentGPUID < 0 {
				curDeploymentGPUID = 0
			}
			goto onemore
		}
		responses[0] = *response
		record.ActualDuration += responses[0].DurationInMicroSec
		responseTime := time.Since(curTime).Milliseconds()
		if responseTime < int64(float64(lastResponseTime)*0.9) {
			curDeploymentGPUID = curDeploymentGPUID + 1
			if curDeploymentGPUID > maxDeploymentGPUID {
				curDeploymentGPUID = maxDeploymentGPUID
			}
		} else if responseTime > int64(float64(lastResponseTime)/0.9) {
			curDeploymentGPUID = curDeploymentGPUID - 1
			if curDeploymentGPUID < 0 {
				curDeploymentGPUID = 0
			}
		}
		lastResponseTime = responseTime
		log.Debugf("invocation name %s, print curReplicas %d, iterations %d ", invocationID, gpu_list[curDeploymentGPUID], curIter)
	}

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Microseconds()

	printDuration := int(record.ActualDuration) / runtimeSpec.Stats.Iterations / 1000
	printResponse := int(int(record.ResponseTime) / runtimeSpec.Stats.Iterations / 1000)
	log.Debugf("**************** HiveDInvoke invocationID %s, actual duration per iteration %d [ms], response Time %d [ms]", invocationID, printDuration, printResponse-printDuration)

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", functions[0].Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", functions[0].Name, float64(record.ResponseTime)/1e3)
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptGradient), sum(responses[0].PromptGradient))

	return true, record
}
