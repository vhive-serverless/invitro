package invokefunc

import (
	"context"
	"fmt"
	"os/exec"
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

type SpeedInfo struct {
	iteration int
	runtime   int64
}

func calPriority(curIter, seconds int) int {
	return curIter / (seconds + 1)
}

func buildHiveDGrpcClients(conn_list []*grpc.ClientConn, functions []*common.Function, runtimeSpec *common.RuntimeSpecification) [][]proto.ExecutorClient {
	grpcClients := make([][]proto.ExecutorClient, len(functions))
	for conn_idx, conn := range conn_list {
		if conn_idx < len(conn_list)-1 {
			grpcClients[conn_idx] = append(grpcClients[conn_idx], proto.NewExecutorClient(conn))
		} else {
			totalBatchSize := runtimeSpec.Stats.BatchSize
			upperboundReplicas := totalBatchSize / common.BszPerDevice * 4
			if upperboundReplicas < common.GPUPerNode {
				grpcClients[conn_idx] = append(grpcClients[conn_idx], proto.NewExecutorClient(conn))
			} else {
				grpcReplicas := upperboundReplicas / common.GPUPerNode
				for i := 0; i < grpcReplicas; i++ {
					grpcClients[conn_idx] = append(grpcClients[conn_idx], proto.NewExecutorClient(conn))
				}
			}
		}
	}
	return grpcClients
}

func prepareLocalGPUSet(upperboundReplicas int, maxGPUPerNode int) []int {
	localGPUSet := make([]int, 0)
	baseGPU := 1
	for baseGPU <= upperboundReplicas {
		localGPUSet = append(localGPUSet, baseGPU)
		baseGPU = baseGPU * 2
	}
	return localGPUSet
}

func lowerboundReplicasToDeadline(remainingService int, deadline int, GPUSet []int) int {
	for _, replicas := range GPUSet {
		jct := int(remainingService / replicas)
		if jct <= deadline {
			return replicas
		}
	}
	return -1
}

func lowerboundReplicasToDeadlineByProfileSpeedMatrix(GPUIteration int, defaultRuntime int, profileSpeedMatrix map[int]SpeedInfo, deadline int, GPUSet []int) int {
	for _, replicas := range GPUSet {
		var jct int
		if speedInfo, ok := profileSpeedMatrix[replicas]; ok {
			jct = GPUIteration / speedInfo.iteration * int(speedInfo.runtime) / replicas
		} else {
			jct = GPUIteration * defaultRuntime / replicas
		}

		if jct <= deadline {
			return min(replicas+1, GPUSet[len(GPUSet)-1])
		}
	}
	return 1 // -1
}

func INFlessInvoke(functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord, *mc.JobExecutionRecord) {
	functionKey := invocationID
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
	trainingIterations := runtimeSpec.Stats.Iterations

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
			record.ResponseTime = time.Since(start).Milliseconds()
			record.ConnectionTimeout = true
			deleteValue(functionKey)
			return true, record, jobRecord
		}
		conn_list[function_idx] = conn
	}
	// register job state into trace scheduler
	setValue(functionKey, runtimeSpec.Stats.BatchSize/common.BszPerDevice)

	for i := 0; i < len(functions); i++ {
		defer gRPCConnectionClose(conn_list[i])
	}

	// executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	leaseTime := 900
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
	// add http header for scheduler
	uuid := uuid.New()
	priority := calPriority(10, 1)
	md := metadata.New(map[string]string{"GPTName": uuid.String(), "RIter": strconv.Itoa(priority)})
	executionCxt = metadata.NewOutgoingContext(executionCxt, md)

	promptTensor := make([]float32, 128*common.EmbedingDim)
	for i := range promptTensor {
		promptTensor[i] = 0
	}

	responses := make([]proto.FaasReply, 32)

	// create grpc clients
	grpcClients := buildHiveDGrpcClients(conn_list, functions, runtimeSpec)
	send_messages := prepareMessages("Can you condense the sentence into a shorter version without losing its meaning?", 100)
	iteration_per_call := trainingIterations

	clusterAvailableGPUs := roundUpToPowerOfTwo(queryRemainingGPU())
	totalBatchSize := runtimeSpec.Stats.BatchSize
	upperboundReplicas := totalBatchSize / common.BszPerDevice // * 4
	// upperboundReplicas := totalBatchSize / common.BszPerDevice
	// lowerboundReplicas := upperboundReplicas
	localGPUSet := prepareLocalGPUSet(upperboundReplicas, common.GPUPerNode)

	specifiedReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice
	lowerboundReplicas := lowerboundReplicasToDeadline(runtimeSpec.Stats.Iterations*runtimeSpec.Runtime*specifiedReplicas, runtimeSpec.Stats.Deadline, localGPUSet)
	lowerboundReplicas = specifiedReplicas
	upperboundReplicas = specifiedReplicas
	initReplicas := specifiedReplicas // roundToPowerOfTwo(max(min(queryFairGPUCount(functionKey), upperboundReplicas), lowerboundReplicas))

	fmt.Printf("invocation name %s, initReplicas %d, upperboundReplicas %d\n", invocationID, initReplicas, upperboundReplicas)
	fmt.Printf("gpu_list %v\n", gpu_list)
	fmt.Printf("prepareLocalGPUSet %v\n", localGPUSet)
	fmt.Printf("clusterAvailableGPUs %v\n", clusterAvailableGPUs)
	maxDeploymentGPUID := findIndex(localGPUSet, initReplicas)
	curDeploymentGPUID := maxDeploymentGPUID
	curIter := 0
	for curIter < trainingIterations {
		// create a wait group to wait for all goroutines to finish
		onceCallStart := time.Now()
		var wg sync.WaitGroup
	onemore:
		doneChan := make(chan struct{})
		deploymentFuncID := min(findIndex(localGPUSet, localGPUSet[curDeploymentGPUID]), len(gpu_list)-1)
		curReplicas := localGPUSet[curDeploymentGPUID]
		grpcReplicas := localGPUSet[curDeploymentGPUID] / gpu_list[deploymentFuncID]
		errorOrNot := false
		for replicaID := 0; replicaID < grpcReplicas; replicaID++ {

			// add one to the wait group
			wg.Add(1)
			// execute the function asynchronously
			go func(replicaID int) {
				defer wg.Done()
				// execute the function and store the response
				response, err := grpcClients[deploymentFuncID][replicaID].Execute(executionCxt, &proto.FaasRequest{
					Message:              send_messages,
					Batchsize:            uint32(common.BszPerDevice),
					RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * iteration_per_call), // [ms]
					GpuMemoryInMebiBytes: 123,
					PromptTensor:         promptTensor,
				})
				if err != nil {
					fmt.Printf("Error executing function: %v\n", err)
					errorOrNot = errorOrNot || true
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
			executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
			// priority := calPriority(curIter, int(time.Since(start).Seconds()))
			priority := 0
			md := metadata.New(map[string]string{"GPTName": uuid.String(), "RIter": strconv.Itoa(priority)})
			executionCxt = metadata.NewOutgoingContext(executionCxt, md)

			log.Debugf("curReplicas %d, lowerboundReplicas %d", curReplicas, lowerboundReplicas)
			allocatedGPUs := roundToPowerOfTwo(max(min(queryFairGPUCount(functionKey), upperboundReplicas), lowerboundReplicas))
			curDeploymentGPUID = findIndex(localGPUSet, allocatedGPUs)

			log.Debugf("gRPC timeout exceeded for INFless invocationID %s, curDeploymentGPUID %d - %s", invocationID, curDeploymentGPUID, "error")
			log.Debugf("**************** gRPC timeout exceeded INFless invocationID %s, curIter %d,  priority %d", invocationID, curIter, priority)

			cmd := exec.Command("kubectl", "get", "pods")
			out, err := cmd.Output()
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Printf("kubectl get pods %s\n", string(out))
			cmd = exec.Command("kubectl", "get", "revisions")
			out, err = cmd.Output()
			if err != nil {
				fmt.Println("Error:", err)
			}
			fmt.Printf("kubectl get revision %s\n", string(out))
			fmt.Printf("**************** time sleep 10 seconds\n")
			time.Sleep(time.Second * 2)
			record.ConnectionTimeout = true
			goto onemore
		}

		registerJobRecord(
			jobRecord,
			onceCallStart.UnixMicro(),
			int64(responses[0].DurationInMicroSec/1e3),
			time.Since(onceCallStart).Milliseconds(),
			localGPUSet[curDeploymentGPUID],
			localGPUSet[curDeploymentGPUID],
			curIter,
			curIter+iteration_per_call,
			trainingIterations,
			runtimeSpec.Stats.BatchSize,
		)
		// update lowerbound replicas to complete it before deadline
		record.ActualDuration += responses[0].DurationInMicroSec / 1e3 // ActualDuration is ms
		curIter += iteration_per_call
		allocatedGPUs := specifiedReplicas
		curDeploymentGPUID = findIndex(localGPUSet, allocatedGPUs)
		log.Debugf("allocate replicas %d, standard replicas %d", allocatedGPUs, totalBatchSize/common.BszPerDevice)
	}

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Milliseconds()
	record.Deadline = runtimeSpec.Stats.Deadline
	record.BatchSize = runtimeSpec.Stats.BatchSize
	record.Iterations = runtimeSpec.Stats.Iterations

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", functions[0].Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", functions[0].Name, float64(record.ResponseTime))
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptGradient), sum(responses[0].PromptGradient))
	cancelExecution()
	deleteValue(functionKey)
	return true, record, jobRecord
}
