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
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
	"github.com/vhive-serverless/loader/pkg/workload/vswarm"
)

func createFakeLoaderConfiguration() *config.LoaderConfiguration {
	return &config.LoaderConfiguration{
		Platform:                     "Knative",
		InvokeProtocol:               "grpc",
		OutputPathPrefix:             "test",
		EnableZipkinTracing:          true,
		GRPCConnectionTimeoutSeconds: 5,
		GRPCFunctionTimeoutSeconds:   15,
	}
}
func createFakeVSwarmLoaderConfiguration() *config.LoaderConfiguration {
	return &config.LoaderConfiguration{
		Platform:                     "Knative",
		InvokeProtocol:               "grpc",
		OutputPathPrefix:             "test",
		EnableZipkinTracing:          true,
		GRPCConnectionTimeoutSeconds: 5,
		GRPCFunctionTimeoutSeconds:   15,
		VSwarm:                       true,
	}
}

var testFunction = common.Function{
	Name: "test-function",
}

var testRuntimeSpecs = common.RuntimeSpecification{
	Runtime: 10, // ms
	Memory:  128,
}

func TestGRPCClientWithServerUnreachable(t *testing.T) {
	cfg := createFakeLoaderConfiguration()
	cfg.EnableZipkinTracing = true

	invoker := CreateInvoker(cfg, nil, nil)
	success, record := invoker.Invoke(&testFunction, &testRuntimeSpecs)

	if record.Instance != "" ||
		record.RequestedDuration != uint32(testRuntimeSpecs.Runtime*1000) ||
		record.StartTime == 0 ||
		record.ResponseTime == 0 ||
		success != false ||
		record.ConnectionTimeout != true {

		t.Error("Error while testing an unreachable server for trace function.")
	}
}

func TestVSwarmClientUnreachable(t *testing.T) {
	cfgSwarm := createFakeVSwarmLoaderConfiguration()

	vSwarmInvoker := CreateInvoker(cfgSwarm, nil, nil)
	success, record := vSwarmInvoker.Invoke(&testFunction, &testRuntimeSpecs)

	if record.Instance != "" ||
		record.RequestedDuration != uint32(testRuntimeSpecs.Runtime*1000) ||
		record.StartTime == 0 ||
		record.ResponseTime == 0 ||
		success != false ||
		record.ConnectionTimeout != true {

		t.Error("Error while testing an unreachable server for vSwarm function.")
	}
}

func TestGRPCClientWithServerReachable(t *testing.T) {
	address, port := "localhost", 18080
	testFunction.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go standard.StartGRPCServer(address, port, standard.TraceFunction, "")

	// make sure that the gRPC server is running
	time.Sleep(2 * time.Second)

	cfg := createFakeLoaderConfiguration()
	invoker := CreateInvoker(cfg, nil, nil)

	start := time.Now()
	success, record := invoker.Invoke(&testFunction, &testRuntimeSpecs)
	logrus.Info("Elapsed: ", time.Since(start).Milliseconds(), " ms")

	if !success ||
		record.MemoryAllocationTimeout != false ||
		record.ConnectionTimeout != false ||
		record.FunctionTimeout != false ||
		record.ResponseTime == 0 ||
		record.ActualDuration == 0 ||
		record.ActualMemoryUsage == 0 {

		t.Error("Failed gRPC invocations for trace function.")
	}
}

func TestVSwarmClientWithServerReachable(t *testing.T) {
	address, port := "localhost", 18081
	testFunction.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go vswarm.StartVSwarmGRPCServer(address, port)
	time.Sleep(2 * time.Second)

	cfgSwarm := createFakeVSwarmLoaderConfiguration()
	vSwarmInvoker := CreateInvoker(cfgSwarm, nil, nil)

	start := time.Now()
	success, record := vSwarmInvoker.Invoke(&testFunction, &testRuntimeSpecs)
	logrus.Info("Elapsed: ", time.Since(start).Milliseconds(), " ms")
	if !success ||
		record.MemoryAllocationTimeout != false ||
		record.ConnectionTimeout != false ||
		record.FunctionTimeout != false ||
		record.ResponseTime == 0 {

		t.Error("Failed gRPC invocations for vSwarm function.")
	}
}

func TestGRPCClientWithServerBatchWorkload(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)
	err := os.Setenv("ITERATIONS_MULTIPLIER", "225")
	if err != nil {
		t.Error(err)
	}

	address, port := "localhost", 18082
	testFunction.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go standard.StartGRPCServer(address, port, standard.TraceFunction, "")

	// make sure that the gRPC server is running
	time.Sleep(2 * time.Second)

	cfg := createFakeLoaderConfiguration()

	invoker := CreateInvoker(cfg, nil, nil)

	for i := 0; i < 50; i++ {
		success, record := invoker.Invoke(&testFunction, &testRuntimeSpecs)

		if !success ||
			record.MemoryAllocationTimeout != false ||
			record.ConnectionTimeout != false ||
			record.FunctionTimeout != false ||
			record.ResponseTime == 0 ||
			record.ActualDuration == 0 ||
			record.ActualMemoryUsage == 0 {

			t.Error("Failed gRPC invocations for trace function.")
		}
	}
}
