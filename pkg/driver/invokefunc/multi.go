package invokefunc

import (
	"context"
	"fmt"
	"os/exec"
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

func MultiInvoke(function *common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord, *mc.JobExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
	}

	jobRecord := &mc.JobExecutionRecord{
		InvocationID:   invocationID,
		StartTime:      make([]int64, 0),
		Replica:        make([]int, 0),
		GpuCount:       make([]int, 0),
		ComputeTime:    make([]int64, 0),
		ExecutionTime:  make([]int64, 0),
		StartIteration: make([]int, 0),
		EndIteration:   make([]int, 0),
		TotalIteration: make([]int, 0),
		BatchSize:      make([]int, 0),
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

	conn, err := grpc.DialContext(dialContext, function.Endpoint, dialOptions...)
	defer gRPCConnectionClose(conn)
	// log.Debugf("SingleInvoke gRPC step 1")
	if err != nil {
		log.Debugf("Failed to establish a gRPC connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return true, record, jobRecord
	}

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	// defer cancelExecution()

	promptTensor := make([]float32, 128)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	// log.Debugf("SingleInvoke gRPC step 2")

	minReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice

	responses := make([]proto.FaasReply, 32)

	// create grpc clients
	grpcClients := make([]proto.ExecutorClient, minReplicas)
	grpcClients[0] = proto.NewExecutorClient(conn)
	// ActualDuration := uint32(0)

	iteration_per_call := 100
	send_messages := prepareMessages("Can you condense the sentence into a shorter version without losing its meaning?", iteration_per_call)

	curIter := 0
	trainingIterations := runtimeSpec.Stats.Iterations
	for curIter < trainingIterations {
		iterStart := time.Now()
	onemore:
		response, err := grpcClients[0].Execute(executionCxt, &proto.FaasRequest{
			Message:              send_messages,
			Batchsize:            uint32(common.BszPerDevice),
			RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * iteration_per_call),
			GpuMemoryInMebiBytes: 123,
			PromptTensor:         promptTensor,
		})

		if err != nil {
			cancelExecution()
			executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
			log.Debugf("gRPC timeout exceeded for SingleInvoke invocationID %s, minReplicas %d - %s", invocationID, minReplicas, err)
			cmd := exec.Command("kubectl", "get", "pods")
			out, err := cmd.Output()
			if err != nil {
				fmt.Println("Error:", err)
			}
			// fmt.Printf("kubectl get pods %s\n", string(out))
			cmd = exec.Command("kubectl", "get", "revisions")
			out, err = cmd.Output()
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Printf("kubectl get revision %s\n", string(out))
			time.Sleep(time.Second * 10)
			goto onemore
		}
		responses[0] = *response
		record.ActualDuration += responses[0].DurationInMicroSec
		registerJobRecord(
			jobRecord,
			iterStart.UnixMicro(),
			int64(responses[0].DurationInMicroSec/1e3),
			time.Since(iterStart).Milliseconds(),
			minReplicas,
			minReplicas,
			curIter,
			curIter+iteration_per_call,
			trainingIterations,
			runtimeSpec.Stats.BatchSize,
		)
		curIter += iteration_per_call
	}

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Microseconds()

	printDuration := int(record.ActualDuration) / runtimeSpec.Stats.Iterations / 1000
	printResponse := int(int(record.ResponseTime) / runtimeSpec.Stats.Iterations / 1000)
	log.Debugf("print minReplicas %d, iterations %d ", minReplicas, runtimeSpec.Stats.Iterations)
	log.Debugf("**************** SingleInvoke invocationID %s, actual duration per iteration %d [ms], response Time %d [ms]", invocationID, printDuration, printResponse-printDuration)

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptGradient), sum(responses[0].PromptGradient))
	cancelExecution()
	return true, record, jobRecord
}