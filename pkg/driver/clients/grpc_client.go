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
	"github.com/google/uuid"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	protoExec "github.com/vhive-serverless/loader/pkg/workload/proto"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"strings"
	"time"
	proto "github.com/vhive-serverless/vSwarm/utils/protobuf/helloworld"

	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	mc "github.com/vhive-serverless/loader/pkg/metric"
)

type grpcInvoker struct {
	cfg *config.LoaderConfiguration
}

func newGRPCInvoker(cfg *config.LoaderConfiguration) *grpcInvoker {
	return &grpcInvoker{
		cfg: cfg,
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
	if i.cfg.EnableZipkinTracing {
		dialOptions = append(dialOptions, grpc.WithStatsHandler(otelgrpc.NewClientHandler()))
	}

	grpcStart := time.Now()

	conn, err := grpc.NewClient(function.Endpoint, dialOptions...)
	if err != nil {
		logrus.Debugf("Failed to establish a gRPC connection - %v\n", err)

		record.ResponseTime = time.Since(start).Microseconds()
		record.ConnectionTimeout = true

		return false, record
	}
	defer gRPCConnectionClose(conn)

	record.GRPCConnectionEstablishTime = time.Since(grpcStart).Microseconds()

	executionCxt, cancelExecution := context.WithTimeout(context.Background(), time.Duration(i.cfg.GRPCFunctionTimeoutSeconds)*time.Second)

	defer cancelExecution()
	if !i.cfg.VSwarm {
		grpcClient := protoExec.NewExecutorClient(conn)
		response, err := grpcClient.Execute(executionCxt, &protoExec.FaasRequest{
			Message:           "nothing",
			RuntimeInMilliSec: uint32(runtimeSpec.Runtime),
			MemoryInMebiBytes: uint32(runtimeSpec.Memory),
		})
		if err != nil {
			logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

			record.ResponseTime = time.Since(start).Microseconds()
			record.FunctionTimeout = true

			return false, record
		}
		record.Instance = extractInstanceName(response.GetMessage())
		record.ResponseTime = time.Since(start).Microseconds()
		record.ActualDuration = response.DurationInMicroSec

		if strings.HasPrefix(response.GetMessage(), "FAILURE - mem_alloc") {
			record.MemoryAllocationTimeout = true
		} else {
			record.ActualMemoryUsage = common.Kib2Mib(response.MemoryUsageInKb)
		}
		logrus.Tracef("(Replied)\t %s: %s, %.2f[ms], %d[MiB]", function.Name, response.Message,
			float64(response.DurationInMicroSec)/1e3, common.Kib2Mib(response.MemoryUsageInKb))
	} else {
		grpcClient := proto.NewGreeterClient(conn)
		response, err := grpcClient.SayHello(executionCxt, &proto.HelloRequest{
			Name: "Invoke Relay",
			VHiveMetadata: MakeVHiveMetadata(
				uuid.New().String(),
				uuid.New().String(),
				time.Now().UTC(),
			),
		})
		if err != nil {
			logrus.Debugf("gRPC timeout exceeded for function %s - %s", function.Name, err)

			record.ResponseTime = time.Since(start).Microseconds()
			record.FunctionTimeout = true

			return false, record
		}
		record.ResponseTime = time.Since(start).Microseconds()
		record.ActualDuration = uint32(record.ResponseTime)
		record.Instance = extractInstanceName(response.GetMessage())
		if strings.HasPrefix(response.GetMessage(), "FAILURE - mem_alloc") {
			record.MemoryAllocationTimeout = true
		} else {
			record.ActualMemoryUsage = common.Kib2Mib(0) //common.Kib2Mib(response.MemoryUsageInKb)
		}
	}

	logrus.Tracef("(E2E Latency) %s: %.2f[ms]\n", function.Name, float64(record.ResponseTime)/1e3)

	return true, record
}

func extractInstanceName(data string) string {
	indexOfHyphen := strings.LastIndex(data, common.FunctionNamePrefix)
	if indexOfHyphen == -1 {
		return data
	}

	return data[indexOfHyphen:]
}

func gRPCConnectionClose(conn *grpc.ClientConn) {
	if conn == nil {
		return
	}

	if err := conn.Close(); err != nil {
		logrus.Warnf("Error while closing gRPC connection - %s\n", err)
	}
}
