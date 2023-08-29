package invokefunc

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func queryRemainingGPU() int {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join("/users/gaow0007", ".kube", "config"))
	if err != nil {
		panic(err.Error())
	}

	// Create a Kubernetes clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	totalGPUs := common.TotalGPUs
	usedGPUs := 0

	// Get the list of Pods in the cluster
	pods, err := clientset.CoreV1().Pods("").List(context.Background(), metav1.ListOptions{FieldSelector: "status.phase=Running"})
	if err != nil {
		panic(err.Error())
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, container := range pod.Spec.Containers {
			if container.Resources.Limits != nil {
				limits := container.Resources.Limits
				if gpu, ok := limits["nvidia.com/gpu"]; ok {
					// fmt.Printf("gpu %v, container %v. gpu Value %d\n", gpu, container.Name, gpu.Value())
					usedGPUs += int(gpu.Value())
				}
			}
		}
	}
	availabeGPUs := totalGPUs - usedGPUs
	fmt.Printf("Total Allocatable GPUs in the cluster: %d\n", availabeGPUs)
	return availabeGPUs
}

func roundToPowerOfTwo(value int) int {
	if value == 0 {
		return 1
	}

	if value&(value-1) == 0 {
		return value
	}
	var exponent uint
	for i := uint(0); i < 32; i++ {
		if 1<<i > value {
			exponent = i - 1
			break
		}
	}
	return 1 << exponent
}

func roundUpToPowerOfTwo(value int) int {
	roundValue := roundToPowerOfTwo(value)
	if value > roundValue {
		roundValue *= 2
	}
	return roundValue 
}

func HiveDInvoke(functions []*common.Function, promptFunctions []*common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {

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

	promptTensor := make([]float32, 128*common.EmbedingDim)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	// log.Debugf("SingleInvoke gRPC step 1")

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
	upperboundReplicas := totalBatchSize / common.BszPerDevice
	lowerboundReplicas := max(upperboundReplicas/2, 1)

	initReplicas := upperboundReplicas
	if clusterAvailableGPUs < initReplicas {
		initReplicas = lowerboundReplicas
	}
	// fmt.Printf("invocation name %s, initReplicas %d, upperboundReplicas %d\n", invocationID, initReplicas, upperboundReplicas)
	maxDeploymentGPUID := findIndex(common.GPUSet, initReplicas)
	curDeploymentGPUID := maxDeploymentGPUID

	curIter := 0
	for curIter < runtimeSpec.Stats.Iterations {
		curTime := time.Now()
	onemore:
		deploymentID := findIndex(gpu_list, common.GPUSet[curDeploymentGPUID])
		curReplicas := common.GPUSet[curDeploymentGPUID]
		accumulationSteps := totalBatchSize / gpu_list[deploymentID] / common.BszPerDevice
		equalIteration := iteration_per_call / accumulationSteps

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
			if curReplicas > lowerboundReplicas {
				curDeploymentGPUID = curDeploymentGPUID - 1
			}

			log.Debugf("gRPC timeout exceeded for SingleInvoke invocationID %s, curDeploymentGPUID %d - %s", invocationID, curDeploymentGPUID, err)
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
		if curIter > 0 {
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
