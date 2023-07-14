package invokefunc

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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
	// return 0
}

func HiveDElasticInvoke(functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {

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

	// executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	leaseTime := 30
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
	// add http header for scheduler
	uuid := uuid.New()
	priority := calPriority(10, 1)
	md := metadata.New(map[string]string{"GPTName": uuid.String(), "RIter": strconv.Itoa(priority)})
	executionCxt = metadata.NewOutgoingContext(executionCxt, md)

	promptTensor := make([]float32, 128)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	// log.Debugf("SingleInvoke gRPC step 1")
	if !strings.Contains(functions[0].Name, "gpt") {
		return false, record
	}

	responses := make([]proto.FaasReply, 32)

	// create grpc clients
	grpcClients := make([]proto.ExecutorClient, len(functions))
	for conn_idx, conn := range conn_list {
		grpcClients[conn_idx] = proto.NewExecutorClient(conn)
	}

	// ActualDuration := uint32(0)

	iteration_per_call := 10
	send_messages := "Can you condense the sentence into a shorter version without losing its meaning?"
	// for i := 0; i < iteration_per_call; i++ {
	// 	for bsz := 0; bsz < common.BszPerDevice; bsz++ {
	// 		send_messages = send_messages + "; Can you condense the sentence into a shorter version without losing its meaning?"
	// 	}
	// }
	clusterAvailableGPUs := roundToPowerOfTwo(queryRemainingGPU())
	totalBatchSize := runtimeSpec.Stats.BatchSize
	upperboundReplicas := min(totalBatchSize/common.BszPerDevice*4, 8)
	lowerboundReplicas := max(totalBatchSize/common.BszPerDevice, 1)

	initReplicas := upperboundReplicas
	if clusterAvailableGPUs < initReplicas {
		initReplicas = lowerboundReplicas
	}
	fmt.Printf("invocation name %s, initReplicas %d, upperboundReplicas %d\n", invocationID, initReplicas, upperboundReplicas)
	maxDeploymentGPUID := findIndex(common.GPUSet, initReplicas)
	curDeploymentGPUID := maxDeploymentGPUID

	curIter := 0
	for curIter < runtimeSpec.Stats.Iterations {
		curTime := time.Now()
	onemore:
		deploymentID := findIndex(gpu_list, common.GPUSet[curDeploymentGPUID])
		curReplicas := common.GPUSet[curDeploymentGPUID]
		equalIteration := 0
		if totalBatchSize/gpu_list[deploymentID] >= common.BszPerDevice {
			accumulationSteps := totalBatchSize / gpu_list[deploymentID] / common.BszPerDevice
			equalIteration = iteration_per_call / accumulationSteps
		} else {
			accumulationSteps := common.BszPerDevice / (totalBatchSize / gpu_list[deploymentID])
			equalIteration = iteration_per_call * accumulationSteps
		}

		response, err := grpcClients[deploymentID].Execute(executionCxt, &proto.FaasRequest{
			Message:              send_messages,
			Batchsize:            uint32(common.BszPerDevice),
			RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * iteration_per_call),
			GpuMemoryInMebiBytes: 123,
			PromptTensor:         promptTensor,
		})

		if err != nil {
			cancelExecution()
			executionCxt, cancelExecution = context.WithTimeout(context.Background(), time.Duration(leaseTime)*time.Second)
			priority := calPriority(curIter, int(time.Since(start).Seconds()))
			md := metadata.New(map[string]string{"GPTName": uuid.String(), "RIter": strconv.Itoa(priority)})
			executionCxt = metadata.NewOutgoingContext(executionCxt, md)

			if curReplicas > lowerboundReplicas {
				curDeploymentGPUID = curDeploymentGPUID - 1
			}

			log.Debugf("gRPC timeout exceeded for HiveDElastic invocationID %s, curDeploymentGPUID %d - %s", invocationID, curDeploymentGPUID, err)
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

		responses[0] = *response
		record.ActualDuration += responses[0].DurationInMicroSec
		responseTime := time.Since(curTime).Milliseconds()

		previousDuration := runtimeSpec.Runtime * iteration_per_call
		if curIter > 0 && curIter%100 == 0 {
			curPrintResponse := int(responseTime)
			curPrintDuration := previousDuration // int(record.ActualDuration) / curIter / 1000 / iteration_per_call
			actualDuration := int(responses[0].DurationInMicroSec / 1000)
			log.Debugf("invocation name %s, print curReplicas %d, maxReplicas %d, iterations %d, totalIterations %d expected duration %d [ms], actualDuration %d [ms], response Time %d [ms]",
				invocationID, gpu_list[curDeploymentGPUID], upperboundReplicas, curIter, runtimeSpec.Stats.Iterations, curPrintDuration, actualDuration, curPrintResponse-actualDuration)
		}

		if curIter%100 == 0 && curIter > 0 && common.GPUSet[curDeploymentGPUID] < upperboundReplicas {
			clusterAvailableGPUs = roundToPowerOfTwo(queryRemainingGPU())
			if clusterAvailableGPUs > upperboundReplicas {
				clusterAvailableGPUs = upperboundReplicas
			}
			if clusterAvailableGPUs >= lowerboundReplicas {
				nextDeploymentGPUID := findIndex(common.GPUSet, clusterAvailableGPUs)
				curRemainingTime := responseTime
				nextRemainingTime := runtimeSpec.Runtime * iteration_per_call / 1000 * common.GPUSet[curDeploymentGPUID] / common.GPUSet[nextDeploymentGPUID]
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
	cancelExecution()
	return true, record
}

// if responseTime < int64(float64(lastResponseTime)*0.9) {
// 	nextDeploymentGPUID := curDeploymentGPUID + 1
// 	if nextDeploymentGPUID > maxDeploymentGPUID {
// 		nextDeploymentGPUID = maxDeploymentGPUID
// 	}
// 	curRemainingTime := responseTime * int64(runtimeSpec.Stats.Iterations/iteration_per_call-curIter)
// 	nextRemainingTime := runtimeSpec.Runtime * iteration_per_call * accumulation_steps / 1000 * common.GPUSet[curDeploymentGPUID] / common.GPUSet[nextDeploymentGPUID]
// 	if int64(float64(nextRemainingTime)*1.1) < curRemainingTime {
// 		curDeploymentGPUID = nextDeploymentGPUID
// 	}

// } else if responseTime > int64(float64(lastResponseTime)/0.9) {
// 	nextDeploymentGPUID := curDeploymentGPUID - 1
// 	if nextDeploymentGPUID < 0 {
// 		nextDeploymentGPUID = 0
// 	}

// 	curRemainingTime := responseTime * int64(runtimeSpec.Stats.Iterations/iteration_per_call-curIter)
// 	nextRemainingTime := runtimeSpec.Runtime * iteration_per_call * accumulation_steps / 1000 * common.GPUSet[curDeploymentGPUID] / common.GPUSet[nextDeploymentGPUID]
// 	if int64(float64(nextRemainingTime)*1.1) < curRemainingTime {
// 		curDeploymentGPUID = nextDeploymentGPUID
// 	}
// } else

// // log.Debugf("previousDuration %d, responseTime %d", previousDuration, responseTime)
// if curIter > 0 && responseTime > int64(float64(previousDuration)*1.1) && curReplicas {
// 	nextDeploymentGPUID := curDeploymentGPUID - 1
// 	if nextDeploymentGPUID < 0 {
// 		nextDeploymentGPUID = 0
// 	}
// 	curRemainingTime := responseTime * int64(runtimeSpec.Stats.Iterations/iteration_per_call-curIter)
// 	nextRemainingTime := runtimeSpec.Runtime * iteration_per_call * accumulationSteps / 1000 * common.GPUSet[curDeploymentGPUID] / common.GPUSet[nextDeploymentGPUID]
// 	if int64(float64(nextRemainingTime)*1.1) < curRemainingTime {
// 		curDeploymentGPUID = nextDeploymentGPUID
// 	}
// 	log.Debugf("update lastResponseTime %d", lastResponseTime)
// }
