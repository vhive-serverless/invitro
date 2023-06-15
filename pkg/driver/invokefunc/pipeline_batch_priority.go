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

func PipelineBatchPriorityInvoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, cfg *config.LoaderConfiguration, invocationID string) (bool, *mc.ExecutionRecord) {
	log.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

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

	minReplicas := runtimeSpec.Stats.BatchSize / common.BszPerDevice
	// minReplicas := 1
	conn_list := make([]*grpc.ClientConn, minReplicas)
	// for replicaID := 0; replicaID < minReplicas; replicaID++ {
	for replicaID := 0; replicaID < 1; replicaID++ {
		conn, err := grpc.DialContext(dialContext, function.Endpoint, dialOptions...)
		conn_list[replicaID] = conn
		if err != nil {
			log.Debugf("Failed to establish a gRPC connection - %v\n", err)
			record.ResponseTime = time.Since(start).Microseconds()
			record.ConnectionTimeout = true

			return false, record
		}
	}
	for i := 0; i < minReplicas; i++ {
		defer gRPCConnectionClose(conn_list[i])
	}

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(cfg.GRPCFunctionTimeoutSeconds)*time.Second)
	defer cancelExecution()
	promptLen := 2
	promptTensor := make([]float32, promptLen)
	for i := range promptTensor {
		promptTensor[i] = 0
	}
	if !strings.Contains(function.Name, "gpt") {
		return false, record
	}

	// add http header for scheduler
	uuid := uuid.New()
	md := metadata.New(map[string]string{"GPTName": uuid.String(), "Replicas": strconv.Itoa(minReplicas), "RIter": strconv.Itoa(runtimeSpec.Stats.Iterations * minReplicas)})
	executionCxt = metadata.NewOutgoingContext(executionCxt, md)

	responses := make([]proto.FaasReply, 32)

	// create a wait group to wait for all goroutines to finish
	var wg sync.WaitGroup

	// create grpc clients
	grpcClients := make([]proto.ExecutorClient, minReplicas)
	for replicaID := 0; replicaID < minReplicas; replicaID++ {

		// grpcClients[replicaID] = proto.NewExecutorClient(conn_list[replicaID])
		// grpcClients[replicaID] = proto.NewExecutorClient(conn)
		grpcClients[replicaID] = proto.NewExecutorClient(conn_list[0])
	}

	ActualDuration := uint32(0)
	send_messages := "C"
	// send_messages := "Can you condense the sentence into a shorter version without losing its meaning?"
	// for i := 0; i < common.BszPerDevice; i++ {
	// 	send_messages = send_messages + "; Can you condense the sentence into a shorter version without losing its meaning?"
	// }
	// iterate over the function iterations

	if minReplicas == 1 {
		response, err := grpcClients[0].Execute(executionCxt, &proto.FaasRequest{
			Message:              send_messages,
			Batchsize:            uint32(common.BszPerDevice),
			RuntimeInMilliSec:    uint32(runtimeSpec.Runtime * runtimeSpec.Stats.Iterations),
			GpuMemoryInMebiBytes: 123,
			PromptTensor:         promptTensor,
		})

		if err != nil {
			log.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)
			record.ResponseTime = time.Since(start).Microseconds()
			record.FunctionTimeout = true
			return false, record
		}
		responses[0] = *response
		ActualDuration += responses[0].DurationInMicroSec
	} else {
		for curIter := 0; curIter < runtimeSpec.Stats.Iterations; curIter++ {
			// create a channel to wait for all function invocations to finish
			doneChan := make(chan struct{})
			if curIter%10 == 0 {
				log.Infof("Function: %s \t exuecte [%d/%d] \t replica [%d] \n", function.Name, curIter, runtimeSpec.Stats.Iterations, minReplicas)
			}
			// cur iteration for promput tuning priority
			md.Set("RIter", strconv.Itoa((runtimeSpec.Stats.Iterations-curIter)*minReplicas))

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
						RuntimeInMilliSec:    uint32(runtimeSpec.Runtime),
						GpuMemoryInMebiBytes: 123,
						PromptTensor:         promptTensor,
					})
					if err != nil {
						fmt.Printf("Error executing function: %v\n", err)
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
			ActualDuration += responses[0].DurationInMicroSec
		}
	}

	// if err != nil {
	// 	log.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

	// 	record.ResponseTime = time.Since(start).Microseconds()
	// 	record.FunctionTimeout = true
	// 	return false, record
	// }

	record.Instance = extractInstanceName(responses[0].GetMessage())
	record.ResponseTime = time.Since(start).Microseconds()
	record.ActualDuration = ActualDuration

	// log.Debugf("PipelineBatchPriority gRPC requested duration %d [ms], actual duration per iteration %d [ms], iteration %d", runtimeSpec.Runtime, int(responses[0].DurationInMicroSec)/runtimeSpec.Stats.Iterations/1000, runtimeSpec.Stats.Iterations)
	printDuration := int(ActualDuration) / runtimeSpec.Stats.Iterations / 1000
	printResponse := int(int(record.ResponseTime) / runtimeSpec.Stats.Iterations / 1000)
	log.Debugf("print minReplicas %d", minReplicas)
	log.Debugf("**************** PipelineBatchPriority gRPC actual duration per iteration %d [ms], response Time %d [ms]", printDuration, printResponse-printDuration)

	if strings.HasPrefix(responses[0].GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(responses[0].GpuMemoryInMebiBytes)
	}

	log.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, responses[0].Message,
		float64(responses[0].DurationInMicroSec)/1e3, responses[0].GpuMemoryInMebiBytes)
	log.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	log.Tracef("Length of Prompt Tensor [%d] \t Sum of Prompt Tensor [%.2f] \n", len(responses[0].PromptGradient), sum(responses[0].PromptGradient))

	return true, record
}