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

func buildGrpcClients(conn *grpc.ClientConn, countOfReplicas int, runtimeSpec *common.RuntimeSpecification) []proto.ExecutorClient {
	grpcClients := make([]proto.ExecutorClient, countOfReplicas)
	for replicaID := 0; replicaID < countOfReplicas; replicaID++ {
		grpcClients[replicaID] = proto.NewExecutorClient(conn)
	}
	return grpcClients
}

func prepareRangeGPUSet(upperboundReplicas int, maxGPUPerNode int) []int {
	localGPUSet := make([]int, 0)
	baseGPU := 1
	for baseGPU <= upperboundReplicas {
		localGPUSet = append(localGPUSet, baseGPU)
		baseGPU = baseGPU + 1
	}
	return localGPUSet
}

func ElasticInvoke(functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {
	functionKey := invocationID
	record := &mc.ExecutionRecord{
		RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
	}
	function := functions[0]
	profileSpeedMatrix := make(map[int]SpeedInfo)
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
	if err != nil {
		log.Debugf("Failed to establish a gRPC connection - %v\n", err)
		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true
		return false, record
	}
	// register job state into trace scheduler
	setValue(functionKey, runtimeSpec.Stats.BatchSize/common.BszPerDevice)

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

	iteration_per_call := 100
	send_messages := prepareMessages("Can you condense the sentence into a shorter version without losing its meaning?", iteration_per_call)
	// iteration_per_call = 100

	clusterAvailableGPUs := queryRemainingGPU()
	totalBatchSize := runtimeSpec.Stats.BatchSize
	upperboundReplicas := totalBatchSize / common.BszPerDevice * 4
	rangeGPUSet := prepareRangeGPUSet(upperboundReplicas, common.GPUPerNode)

	specifiedReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice
	lowerboundReplicas := lowerboundReplicasToDeadline(runtimeSpec.Stats.Iterations*runtimeSpec.Runtime*specifiedReplicas, runtimeSpec.Stats.Deadline, rangeGPUSet)

	// lowerboundReplicas = specifiedReplicas
	// upperboundReplicas = specifiedReplicas
	initReplicas := specifiedReplicas // roundToPowerOfTwo(max(min(queryFairGPUCount(functionKey), upperboundReplicas), lowerboundReplicas))

	// create grpc clients
	grpcClients := buildGrpcClients(conn, upperboundReplicas, runtimeSpec)

	fmt.Printf("invocation name %s, initReplicas %d, upperboundReplicas %d\n", invocationID, initReplicas, upperboundReplicas)
	fmt.Printf("prepareLocalGPUSet %v\n", rangeGPUSet)
	fmt.Printf("clusterAvailableGPUs %v\n", clusterAvailableGPUs)
	maxDeploymentGPUID := findIndex(rangeGPUSet, initReplicas)
	curDeploymentGPUID := maxDeploymentGPUID
	curIter := 0
	for curIter < runtimeSpec.Stats.Iterations {
		// create a wait group to wait for all goroutines to finish
		onceCallStart := time.Now()
		var wg sync.WaitGroup
	onemore:
		doneChan := make(chan struct{})
		curReplicas := rangeGPUSet[curDeploymentGPUID]
		equalIteration := 0
		cur_iteration_per_call := min(iteration_per_call, runtimeSpec.Stats.Iterations-curIter)
		if totalBatchSize/rangeGPUSet[curDeploymentGPUID] >= common.BszPerDevice {
			accumulationSteps := totalBatchSize / rangeGPUSet[curDeploymentGPUID] / common.BszPerDevice
			cur_iteration_per_call = cur_iteration_per_call * accumulationSteps // to avoid a float progress
			equalIteration = cur_iteration_per_call / accumulationSteps
		} else {
			accumulationSteps := common.BszPerDevice / (totalBatchSize / rangeGPUSet[curDeploymentGPUID])
			if cur_iteration_per_call*accumulationSteps+curIter > runtimeSpec.Stats.Iterations {
				cur_iteration_per_call = (runtimeSpec.Stats.Iterations - curIter) / accumulationSteps
			}
			equalIteration = cur_iteration_per_call * accumulationSteps
		}
		log.Debugf("############### invocation name %s, cur_iteration_per_call %d, equalIteration %d, curIteration %d, totalIteration %d",
			invocationID, cur_iteration_per_call, equalIteration, curIter, runtimeSpec.Stats.Iterations)

		grpcReplicas := rangeGPUSet[curDeploymentGPUID]
		errorOrNot := false
		for replicaID := 0; replicaID < grpcReplicas; replicaID++ {

			// add one to the wait group
			wg.Add(1)
			// execute the function asynchronously
			go func(replicaID int) {
				defer wg.Done()
				// execute the function and store the response
				response, err := grpcClients[replicaID].Execute(executionCxt, &proto.FaasRequest{
					Message:              send_messages,
					Batchsize:            uint32(common.BszPerDevice),
					RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * cur_iteration_per_call), // [ms]
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
			allocatedGPUs := max(min(queryFairGPUCount(functionKey), upperboundReplicas), lowerboundReplicas)
			curDeploymentGPUID = findIndex(rangeGPUSet, allocatedGPUs)

			log.Debugf("gRPC timeout exceeded for Elastic invocationID %s, curDeploymentGPUID %d - %s", invocationID, curDeploymentGPUID, "error")
			log.Debugf("**************** gRPC timeout exceeded ElasticInvoke invocationID %s, curIter %d,  priority %d", invocationID, curIter, priority)

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
		// update speed info matrix
		callRuntime := time.Since(onceCallStart).Milliseconds()
		if speedInfo, ok := profileSpeedMatrix[rangeGPUSet[curDeploymentGPUID]]; ok {
			speedInfo.iteration = cur_iteration_per_call
			speedInfo.runtime = callRuntime
			profileSpeedMatrix[rangeGPUSet[curDeploymentGPUID]] = speedInfo
		} else {
			profileSpeedMatrix[rangeGPUSet[curDeploymentGPUID]] = SpeedInfo{
				iteration: cur_iteration_per_call,
				runtime:   callRuntime,
			}
		}
		// update lowerbound replicas to complete it before deadline
		record.ActualDuration += responses[0].DurationInMicroSec / 1e3 // ActualDuration is ms
		curIter += equalIteration
		allocatedGPUs := specifiedReplicas
		curDeploymentGPUID = findIndex(rangeGPUSet, allocatedGPUs)
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
	return true, record
}
