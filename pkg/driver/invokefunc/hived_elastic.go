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

func calPriority(curIter, seconds int) int {
	return curIter / (seconds + 1)
}

func buildGrpcClients(conn_list []*grpc.ClientConn, functions []*common.Function, runtimeSpec *common.RuntimeSpecification) [][]proto.ExecutorClient {
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

func HiveDElasticInvoke(functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {
	record := &mc.ExecutionRecord{
		RequestedDuration: uint32(0),
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
			record.ResponseTime = time.Since(start).Milliseconds()
			record.ConnectionTimeout = true
			return false, record
		}
		conn_list[function_idx] = conn
	}

	for i := 0; i < len(functions); i++ {
		defer gRPCConnectionClose(conn_list[i])
	}

	// executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	leaseTime := 30
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
	grpcClients := buildGrpcClients(conn_list, functions, runtimeSpec)
	iteration_per_call := 10
	send_messages := prepareMessages("Can you condense the sentence into a shorter version without losing its meaning?", iteration_per_call*common.BszPerDevice)

	clusterAvailableGPUs := roundToPowerOfTwo(queryRemainingGPU())
	totalBatchSize := runtimeSpec.Stats.BatchSize
	upperboundReplicas := totalBatchSize / common.BszPerDevice * 4
	localGPUSet := prepareLocalGPUSet(upperboundReplicas, common.GPUPerNode)

	specifiedReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice
	lowerboundReplicas := specifiedReplicas
	for _, replicas := range localGPUSet {
		jct := int(runtimeSpec.Stats.Iterations * runtimeSpec.Runtime * specifiedReplicas / replicas)
		if jct <= runtimeSpec.Stats.Deadline {
			lowerboundReplicas = replicas
			break
		}
	}
	initReplicas := lowerboundReplicas

	fmt.Printf("invocation name %s, initReplicas %d, upperboundReplicas %d\n", invocationID, initReplicas, upperboundReplicas)
	fmt.Printf("gpu_list %v\n", gpu_list)
	fmt.Printf("prepareLocalGPUSet %v\n", localGPUSet)
	fmt.Printf("clusterAvailableGPUs %v\n", clusterAvailableGPUs)
	maxDeploymentGPUID := findIndex(localGPUSet, initReplicas)
	curDeploymentGPUID := maxDeploymentGPUID

	curIter := 0
	for curIter < runtimeSpec.Stats.Iterations {
		// create a wait group to wait for all goroutines to finish
		var wg sync.WaitGroup
		curTime := time.Now()
	onemore:
		doneChan := make(chan struct{})
		deploymentFuncID := min(findIndex(localGPUSet, localGPUSet[curDeploymentGPUID]), len(gpu_list)-1)
		curReplicas := localGPUSet[curDeploymentGPUID]
		equalIteration := 0
		if totalBatchSize/localGPUSet[curDeploymentGPUID] >= common.BszPerDevice {
			accumulationSteps := totalBatchSize / localGPUSet[curDeploymentGPUID] / common.BszPerDevice
			equalIteration = iteration_per_call / accumulationSteps
		} else {
			accumulationSteps := common.BszPerDevice / (totalBatchSize / localGPUSet[curDeploymentGPUID])
			equalIteration = iteration_per_call * accumulationSteps
		}

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
			priority := calPriority(curIter, int(time.Since(start).Seconds()))
			md := metadata.New(map[string]string{"GPTName": uuid.String(), "RIter": strconv.Itoa(priority)})
			executionCxt = metadata.NewOutgoingContext(executionCxt, md)

			log.Debugf("curReplicas %d, lowerboundReplicas %d", curReplicas, lowerboundReplicas)
			if curReplicas > lowerboundReplicas {
				curDeploymentGPUID = curDeploymentGPUID - 1
			}

			log.Debugf("gRPC timeout exceeded for HiveDElastic invocationID %s, curDeploymentGPUID %d - %s", invocationID, curDeploymentGPUID, "error")
			log.Debugf("**************** gRPC timeout exceeded HiveDInvoke invocationID %s, curIter %d,  priority %d", invocationID, curIter, priority)

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

		record.ActualDuration += responses[0].DurationInMicroSec / 1e3 // ActualDuration is ms
		responseTime := time.Since(curTime).Milliseconds()

		previousDuration := runtimeSpec.Runtime * iteration_per_call
		if curIter > 0 && curIter%100 == 0 {
			curPrintResponse := int(responseTime)
			curPrintDuration := previousDuration // int(record.ActualDuration) / curIter / 1000 / iteration_per_call
			actualDuration := int(responses[0].DurationInMicroSec / 1000)
			log.Debugf("invocation name %s, print curReplicas %d, maxReplicas %d, iterations %d, totalIterations %d expected duration %d [ms], actualDuration %d [ms], response Time %d [ms]",
				invocationID, localGPUSet[curDeploymentGPUID], upperboundReplicas, curIter, runtimeSpec.Stats.Iterations, curPrintDuration, actualDuration, curPrintResponse-actualDuration)
		}

		if curIter%100 == 0 && curIter > 0 && localGPUSet[curDeploymentGPUID] < upperboundReplicas {
			clusterAvailableGPUs = roundToPowerOfTwo(queryRemainingGPU())
			if clusterAvailableGPUs > upperboundReplicas {
				clusterAvailableGPUs = upperboundReplicas
			}
			if clusterAvailableGPUs >= lowerboundReplicas {
				nextDeploymentGPUID := findIndex(localGPUSet, clusterAvailableGPUs)
				curRemainingTime := responseTime
				nextRemainingTime := runtimeSpec.Runtime * iteration_per_call / 1000 * localGPUSet[curDeploymentGPUID] / localGPUSet[nextDeploymentGPUID]
				if int64(float64(nextRemainingTime)*1.1) < curRemainingTime {
					curDeploymentGPUID = nextDeploymentGPUID
				}
			}

		}

		if curIter%100 == 0 && curIter > 0 {
			cancelExecution()
			executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
			priority := calPriority(curIter, int(time.Since(start).Seconds()))
			md.Set("RIter", strconv.Itoa((priority)))
			executionCxt = metadata.NewOutgoingContext(executionCxt, md)
			log.Debugf("**************** HiveDInvoke invocationID %s, curIter %d,  priority %d", invocationID, curIter, priority)
		}
		curIter += equalIteration
	}

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Milliseconds()
	record.Deadline = runtimeSpec.Stats.Deadline

	printDuration := int(record.ActualDuration) / runtimeSpec.Stats.Iterations
	printResponse := int(int(record.ResponseTime) / runtimeSpec.Stats.Iterations)
	log.Debugf("**************** HiveDInvoke invocationID %s, actual duration per iteration %d [ms], response Time %d [ms]", invocationID, printDuration, printResponse-printDuration)

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
	return true, record
}
