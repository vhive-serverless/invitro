/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package clients

import (
	"context"
	"math"

	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/workload/proto"

	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type invoker interface {
	Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context, prefetchBucket string, prefetchKey []string) bool
}

type ExecutorRPC struct {
}

var FunctionTimeouts = map[string]float64{
	"chameleonserve": 80.62, "cnnserve": 481.005, "imageresize": 2070.765,
	"lrserving": 106.3495, "mapper": 809.065, "pyaesserve": 55.638,
	"reducer": 4935.275, "rnnserve": 101.7505, "streducer": 312.2645,
	"sttrainer": 213.7305,
}

var FunctionPayloads = map[string][]string{
	"chameleonserve": {"input_payload/chameleon/chameleon_input.txt"},
	"cnnserve":       {"input_payload/cnn_serving/cnn_input.jpg"},
	"imageresize":    {"input_payload/image_resize/image_resize_input.jpg"},
	"lrserving": {"input_payload/lr_serving/lr_serving_input.txt", "input_payload/lr_serving/lr_serving_tokenizer.pkl",
		"input_payload/lr_serving/lr_serving_scaler.pkl", "input_payload/lr_serving/lr_serving_model.pkl"},
	"mapper":     {"input_payload/mapper/part-00000.csv"},
	"pyaesserve": {"input_payload/pyaes/pyaes_input.txt"},
	"reducer": {"input_payload/reducer/part-00000.json", "input_payload/reducer/part-00001.json", "input_payload/reducer/part-00002.json",
		"input_payload/reducer/part-00003.json", "input_payload/reducer/part-00004.json", "input_payload/reducer/part-00005.json",
		"input_payload/reducer/part-00006.json", "input_payload/reducer/part-00007.json"},
	"rnnserve": {"input_payload/rnn_serving/rnn_serving_input.txt"},
	"streducer": {"input_payload/stack_training-reducer/KNeighborsRegressor.pkl", "input_payload/stack_training-reducer/KNeighborsRegressor_y_pred.pkl",
		"input_payload/stack_training-reducer/Lasso.pkl", "input_payload/stack_training-reducer/Lasso_y_pred.pkl",
		"input_payload/stack_training-reducer/LinearRegression.pkl", "input_payload/stack_training-reducer/LinearRegression_y_pred.pkl",
		"input_payload/stack_training-reducer/LinearSVR.pkl", "input_payload/stack_training-reducer/LinearSVR_y_pred.pkl",
		"input_payload/stack_training-reducer/RandomForestRegressor.pkl", "input_payload/stack_training-reducer/RandomForestRegressor_y_pred.pkl"},
	"sttrainer": {"input_payload/stack_training-trainer/dataset"},
}

func (i ExecutorRPC) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context, prefetchBucket string, prefetchKey []string) bool {
	grpcClient := proto.NewExecutorClient(conn)

	response, err := grpcClient.Execute(executionCxt, &proto.FaasRequest{
		Message:           "nothing",
		RuntimeInMilliSec: uint32(runtimeSpec.Runtime),
		MemoryInMebiBytes: uint32(runtimeSpec.Memory),
	})

	if err != nil {
		logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

		record.ConnectionTimeout = true // WithBlock deprecated in new gRPC interface
		record.FunctionTimeout = true

		return false
	}

	record.Instance = extractInstanceName(response.GetMessage())
	record.ActualDuration = response.DurationInMicroSec

	if strings.HasPrefix(response.GetMessage(), "FAILURE - mem_alloc") {
		record.MemoryAllocationTimeout = true
	} else {
		record.ActualMemoryUsage = common.Kib2Mib(response.MemoryUsageInKb)
	}

	logrus.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
		float64(response.DurationInMicroSec)/1e3, common.Kib2Mib(response.MemoryUsageInKb))

	return true
}

type SayHelloRPC struct {
}

func (i SayHelloRPC) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context, prefetchBucket string, prefetchKey []string) bool {
	grpcClient := proto.NewNexusRPCServerClient(conn)
	response, err := grpcClient.NexusRPC(executionCxt, &proto.NexusRPCRequest{
		Msg:     "",
		Payload: []byte("Hello"),
	})
	if err != nil {
		logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)
		record.ConnectionTimeout = true
		record.FunctionTimeout = true

		return false
	}
	record.ActualDuration = 0
	record.Instance = response.GetMsg()
	record.ActualMemoryUsage = common.Kib2Mib(0) //Memory usage may not be available for all vSwarm benchmarks

	return true
}

type NexusRPC struct {
	cfg     *config.LoaderConfiguration
	invoker invoker
}

func NewNexusRPC(cfg *config.LoaderConfiguration, invoker invoker) *NexusRPC {
	return &NexusRPC{
		cfg:     cfg,
		invoker: invoker,
	}
}

func (i NexusRPC) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification, conn *grpc.ClientConn, record *mc.ExecutionRecord, executionCxt context.Context, prefetchBucket string, prefetchKey []string) bool {
	grpcClient := proto.NewNexusRPCServerClient(conn)
	response, err := grpcClient.NexusRPC(executionCxt, &proto.NexusRPCRequest{
		Msg:            "",
		Payload:        []byte("Hello"),
		PrefetchBucket: prefetchBucket,
		PrefetchKey:    prefetchKey,
	})

	// response, err := grpcClient.NexusRPC(executionCxt, &proto.NexusRPCRequest{
	// 	Msg:     "",
	// 	Payload: []byte("Hello"),
	// })

	if err != nil {
		logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)
		record.ConnectionTimeout = true
		record.FunctionTimeout = true

		return false
	}
	record.ActualDuration = 0
	record.Instance = extractSwarmFunction(response.GetMsg())
	record.ActualMemoryUsage = common.Kib2Mib(0) //Memory usage may not be available for all vSwarm benchmarks

	return true
}

type grpcInvoker struct {
	cfg     *config.LoaderConfiguration
	invoker invoker
}

func newGRPCInvoker(cfg *config.LoaderConfiguration, invoker invoker) *grpcInvoker {
	return &grpcInvoker{
		cfg:     cfg,
		invoker: invoker,
	}
}

func (i *grpcInvoker) Invoke(function *common.Function, runtimeSpec *common.RuntimeSpecification) (bool, *mc.ExecutionRecord) {
	logrus.Tracef("(Invoke)\t %s: %d[ms], %d[MiB]", function.Name, runtimeSpec.Runtime, runtimeSpec.Memory)

	record := &mc.ExecutionRecord{
		ExecutionRecordBase: mc.ExecutionRecordBase{
			RequestedDuration: uint32(runtimeSpec.Runtime * 1e3),
		},
	}

	////////////////////////////////////
	// INVOKE FUNCTION
	////////////////////////////////////
	start := time.Now()
	record.StartTime = start.UnixMicro()

	var dialOptions []grpc.DialOption
	dialOptions = append(dialOptions, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if strings.Contains(i.cfg.Platform, common.PlatformDirigent) {
		dialOptions = append(dialOptions, grpc.WithAuthority(function.Name)) // Dirigent specific
	}
	if i.cfg.EnableZipkinTracing {
		dialOptions = append(dialOptions, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	}
	var prefetchBucket string
	var prefetchKeys []string
	if i.cfg.EnablePrefetch {
		log.Tracef("Prefetch enabled for function %s", function.Name)
		prefetchBucket, prefetchKeys = perWorkloadPrefetchKeys(function)
	}

	grpcStart := time.Now()

	conn, err := grpc.NewClient("passthrough:///"+function.Endpoint, dialOptions...)
	if err != nil {
		logrus.Debugf("Failed to establish a gRPC connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}
	defer gRPCConnectionClose(conn)

	record.GRPCConnectionEstablishTime = time.Since(grpcStart).Microseconds()
	executionCxt, cancelExecution := context.WithTimeout(context.Background(), perFunctionTimeout(i.cfg, function))
	defer cancelExecution()

	success := i.invoker.Invoke(function, runtimeSpec, conn, record, executionCxt, prefetchBucket, prefetchKeys)
	record.ResponseTime = time.Since(start).Microseconds()
	logrus.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)
	return success, record
}

func perFunctionTimeout(cfg *config.LoaderConfiguration, function *common.Function) time.Duration {
	// map of function name to timeout values can be added here
	// split function name by '-' and get the first part

	parsedName := strings.Split(function.Name, "-")[0]
	if timeout, ok := FunctionTimeouts[parsedName]; ok {
		SLO := float64(10)
		newTimeout := time.Duration(math.Min(timeout*SLO, 20*1000) * float64(time.Millisecond))
		log.Tracef("Using custom timeout for function %s: %.2f seconds", function.Name, newTimeout.Seconds())
		return newTimeout
	} else {
		newTimeout := time.Duration(cfg.GRPCFunctionTimeoutSeconds) * time.Second
		log.Tracef("Using default timeout for function %s: %d seconds", function.Name, cfg.GRPCFunctionTimeoutSeconds)
		return newTimeout
	}
	// return time.Duration(cfg.GRPCFunctionTimeoutSeconds) * time.Second
}

func perWorkloadPrefetchKeys(function *common.Function) (string, []string) {
	parsedName := strings.Split(function.Name, "-")[0]
	if payloads, ok := FunctionPayloads[parsedName]; ok {
		log.Tracef("Using prefetch payloads for function %s: %v", function.Name, payloads)
		return "nexus-benchmark-payload", payloads
	} else {
		log.Tracef("No prefetch payloads found for function %s", function.Name)
		return "", []string{}
	}
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, common.FunctionNamePrefix)
	if indexOfHyphen == -1 {
		return data
	}

	return data[indexOfHyphen:]
}

func extractSwarmFunction(data string) string {
	index := strings.Index(data, "fn: ")
	verticalBarIndex := strings.Index(data, " |")
	if index == -1 {
		return data
	}
	if verticalBarIndex == -1 {
		return data[index+4:]
	}
	return data[index+4 : verticalBarIndex]
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		logrus.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}
