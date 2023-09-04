package invokefunc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	"github.com/eth-easl/loader/pkg/workload/proto"
	"github.com/google/uuid"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"

	mc "github.com/eth-easl/loader/pkg/metric"
)

func BatchInvoke(function *common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord, *mc.JobExecutionRecord) {
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

	initPromptTensor := make([]float32, 128*common.EmbedingDim)
	iterationScale := float32(1.0)
	trainingIterations := runtimeSpec.Stats.Iterations
	if cfg.WithPromptBank {
		initPromptTensor, iterationScale = PromptBankInvoke(
			promptFunctions,
			runtimeSpec,
			cfg,
			invocationID,
		)
		trainingIterations = int(float32(trainingIterations) * iterationScale)
	}

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
	if err != nil {
		log.Debugf("Failed to establish a gRPC connection - %v\n", err)
		record.ResponseTime = time.Since(start).Milliseconds()
		record.ConnectionTimeout = true
		return false, record, jobRecord
	}

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	defer cancelExecution()

	promptTensor := make([]float32, 128*common.EmbedingDim)
	if cfg.WithPromptBank {
		for i := range promptTensor {
			promptTensor[i] = initPromptTensor[i]
		}
	} else {
		for i := range promptTensor {
			promptTensor[i] = 0
		}
	}

	minReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice
	// add http header for scheduler
	uuid := uuid.New()
	md := metadata.New(map[string]string{"GPTName": uuid.String(), "Replicas": strconv.Itoa(minReplicas), "RIter": "0", "cur": time.Now().Format("2006-01-02 15:04:05.999")})
	executionCxt = metadata.NewOutgoingContext(executionCxt, md)

	responses := make([]proto.FaasReply, 32)

	// create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// create grpc clients
	grpcClients := make([]proto.ExecutorClient, minReplicas)
	for replicaID := 0; replicaID < minReplicas; replicaID++ {
		grpcClients[replicaID] = proto.NewExecutorClient(conn)
	}

	ActualDuration := uint32(0)

	iteration_per_call := 100
	send_messages := prepareMessages("Can you condense the sentence into a shorter version without losing its meaning?", iteration_per_call)

	log.Debugf("deadline is %d [ms]", runtimeSpec.Stats.Deadline)
	// iterate over the function iterations
	curIter := 0
	for curIter < trainingIterations {
		iterStart := time.Now()
	onemore:
		md.Set("cur", time.Now().Format("2006-01-02 15:04:05.999"))
		// create a channel to wait for all function invocations to finish
		doneChan := make(chan struct{})
		if curIter%100 == 0 {
			log.Infof("Function: %s \t exuecte [%d/%d] \t replica [%d] \n", invocationID, curIter, runtimeSpec.Stats.Iterations, minReplicas)
		}
		// cur iteration for promput tuning priority
		// md.Set("RIter", strconv.Itoa(runtimeSpec.Stats.Iterations-curIter))

		errorOrNot := false
		errorMessage := ""
		// iterate over the minimum replicas
		for replicaID := 0; replicaID < minReplicas; replicaID++ {
			// add one to the wait group
			wg.Add(1)
			// execute the function asynchronously
			go func(replicaID int) {
				defer wg.Done()
				// execute the function and store the response
				response, err := grpcClients[replicaID].Execute(executionCxt, &proto.FaasRequest{
					Message:              send_messages,
					Batchsize:            uint32(common.BszPerDevice),
					RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * iteration_per_call), // ms
					GpuMemoryInMebiBytes: 123,
					PromptTensor:         promptTensor,
				})
				if err != nil {
					errorOrNot = errorOrNot || true
					errorMessage = fmt.Sprintf("Error executing function: %v for replicaID %d\n", err, replicaID)
					return
				}

				// store the response in the slice
				responses[replicaID] = *response
			}(replicaID)
		}

		// create a goroutine to wait for all goroutines to finish
		go func() {
			wg.Wait()
			close(doneChan)
		}()
		// wait for all function invocations to finish
		<-doneChan

		if errorOrNot {
			cancelExecution()
			executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
			executionCxt = metadata.NewOutgoingContext(executionCxt, md)
			log.Debugf("gRPC timeout exceeded for batch scheduler in invocationID %s - %s", invocationID, errorMessage)
			time.Sleep(time.Second * 10)
			record.ConnectionTimeout = true
			goto onemore
		}

		// gradient average
		promptTensor = responses[0].PromptGradient
		for i := 1; i < len(responses); i++ {
			for j := 0; j < len(promptTensor); j++ {
				promptTensor[j] += responses[0].PromptGradient[j]
			}
		}

		for j := 0; j < len(promptTensor); j++ {
			promptTensor[j] = promptTensor[j] / float32(len(responses))
		}
		ActualDuration += responses[0].DurationInMicroSec / 1e3
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
	record.ResponseTime = time.Since(start).Milliseconds()
	record.Deadline = runtimeSpec.Stats.Deadline
	record.BatchSize = runtimeSpec.Stats.BatchSize
	record.Iterations = runtimeSpec.Stats.Iterations
	record.ActualDuration = ActualDuration // ActualDuration is MicroSec
	log.Debugf("gRPC requested duration %d [ms], actual duration per iteration %d [ms], iteration %d", runtimeSpec.Runtime, int(ActualDuration)/runtimeSpec.Stats.Iterations/1000, runtimeSpec.Stats.Iterations)

	// log.Debugf("PipelineBatchPriority gRPC requested duration %d [ms], actual duration per iteration %d [ms], iteration %d", runtimeSpec.Runtime, int(responses[0].DurationInMicroSec)/runtimeSpec.Stats.Iterations/1000, runtimeSpec.Stats.Iterations)
	// printDuration := int(ActualDuration) / runtimeSpec.Stats.Iterations
	// printResponse := int(int(record.ResponseTime) / runtimeSpec.Stats.Iterations)
	// log.Debugf("print minReplicas %d, iterations %d ", minReplicas, runtimeSpec.Stats.Iterations)
	// log.Debugf("**************** On Batch gRPC actual duration per iteration %d [ms], response Time %d [ms]", printDuration, printResponse-printDuration)

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime))
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptGradient), sum(responses[0].PromptGradient))
	cancelExecution()
	return true, record, jobRecord
}
