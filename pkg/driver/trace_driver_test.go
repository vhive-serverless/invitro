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

package driver

import (
	"container/list"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/pkg/config"

	"github.com/gocarina/gocsv"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/metric"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
	"github.com/vhive-serverless/loader/pkg/workload/vswarm"
)

func createFakeLoaderConfiguration(vSwarm bool) *config.LoaderConfiguration {
	return &config.LoaderConfiguration{
		Platform:                     "Knative",
		InvokeProtocol:               "grpc",
		OutputPathPrefix:             "test",
		EnableZipkinTracing:          true,
		GRPCConnectionTimeoutSeconds: 5,
		GRPCFunctionTimeoutSeconds:   15,
		VSwarm:                       vSwarm,
	}
}

func createTestDriver(invocationStats []int, vSwarm bool) *Driver {
	cfg := createFakeLoaderConfiguration(vSwarm)

	driver := NewDriver(&config.Configuration{
		LoaderConfiguration: cfg,
		IATDistribution:     common.Equidistant,
		TraceDuration:       1,

		Functions: []*common.Function{
			{
				Name: "test-function",
				InvocationStats: &common.FunctionInvocationStats{
					Invocations: invocationStats,
				},
				RuntimeStats: &common.FunctionRuntimeStats{
					Average:       50,
					Count:         100,
					Minimum:       0,
					Maximum:       100,
					Percentile0:   0,
					Percentile1:   1,
					Percentile25:  25,
					Percentile50:  50,
					Percentile75:  75,
					Percentile99:  99,
					Percentile100: 100,
				},
				MemoryStats: &common.FunctionMemoryStats{
					Average:       5000,
					Count:         100,
					Percentile1:   100,
					Percentile5:   500,
					Percentile25:  2500,
					Percentile50:  5000,
					Percentile75:  7500,
					Percentile95:  9500,
					Percentile99:  9900,
					Percentile100: 10000,
				},
				Specification: &common.FunctionSpecification{
					PerMinuteCount: invocationStats,
				},
			},
		},
		TestMode: true,
	})

	return driver
}

func TestInvokeFunctionFromDriver(t *testing.T) {
	tests := []struct {
		testName  string
		port      int
		forceFail bool
	}{
		{
			testName:  "invoke_failure",
			port:      8082,
			forceFail: true,
		},
		{
			testName:  "invoke_success",
			port:      8083,
			forceFail: false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			var successCount int64 = 0
			var failureCount int64 = 0

			invocationRecordOutputChannel := make(chan *metric.ExecutionRecord, 1)
			announceDone := &sync.WaitGroup{}

			testDriver := createTestDriver([]int{1}, false)
			var functionsInvoked int64
			if !test.forceFail {
				address, port := "localhost", test.port
				testDriver.Configuration.Functions[0].Endpoint = fmt.Sprintf("%s:%d", address, port)

				go standard.StartGRPCServer(address, port, standard.TraceFunction, "")

				// make sure that the gRPC server is running
				time.Sleep(2 * time.Second)
			}
			function := testDriver.Configuration.Functions[0]
			node := &common.Node{Function: testDriver.Configuration.Functions[0]}
			list := list.New()
			list.PushBack(node)
			function.Specification.RuntimeSpecification = []common.RuntimeSpecification{{
				Runtime: 1000,
				Memory:  128,
			}}
			metadata := &InvocationMetadata{
				RootFunction:        list,
				Phase:               common.ExecutionPhase,
				IatIndex:            0,
				InvocationID:        composeInvocationID(common.MinuteGranularity, 0, 0),
				SuccessCount:        &successCount,
				FailedCount:         &failureCount,
				FunctionsInvoked:    &functionsInvoked,
				RecordOutputChannel: invocationRecordOutputChannel,
				AnnounceDoneWG:      announceDone,
			}

			announceDone.Add(1)
			testDriver.invokeFunction(metadata)

			switch test.forceFail {
			case true:
				if !(successCount == 0 && failureCount == 1 && functionsInvoked == 1) {
					t.Error("The function somehow managed to execute.")
				}
			case false:
				if !(successCount == 1 && failureCount == 0 && functionsInvoked == 1) {
					t.Error("The function should not have failed.")
				}
			}

			record := <-invocationRecordOutputChannel
			announceDone.Wait()

			if record.Phase != int(metadata.Phase) ||
				record.InvocationID != composeInvocationID(common.MinuteGranularity, 0, 0) {

				t.Error("Invalid invocation record received.")
			}
		})
	}
}

func TestVSwarmInvokeFunctionFromDriver(t *testing.T) {
	tests := []struct {
		testName  string
		port      int
		forceFail bool
	}{
		{
			testName:  "invoke_failure",
			port:      8084,
			forceFail: true,
		},
		{
			testName:  "invoke_success",
			port:      8085,
			forceFail: false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			var successCount int64 = 0
			var failureCount int64 = 0

			invocationRecordOutputChannel := make(chan *metric.ExecutionRecord, 1)
			announceDone := &sync.WaitGroup{}

			testDriver := createTestDriver([]int{1}, true)
			var functionsInvoked int64
			if !test.forceFail {
				address, port := "localhost", test.port
				testDriver.Configuration.Functions[0].Endpoint = fmt.Sprintf("%s:%d", address, port)

				go vswarm.StartVSwarmGRPCServer(address, port)

				// make sure that the gRPC server is running
				time.Sleep(2 * time.Second)
			}
			function := testDriver.Configuration.Functions[0]
			node := &common.Node{Function: testDriver.Configuration.Functions[0]}
			list := list.New()
			list.PushBack(node)
			function.Specification.RuntimeSpecification = []common.RuntimeSpecification{{
				Runtime: 1000,
				Memory:  128,
			}}
			metadata := &InvocationMetadata{
				RootFunction:        list,
				Phase:               common.ExecutionPhase,
				IatIndex:            0,
				InvocationID:        composeInvocationID(common.MinuteGranularity, 0, 0),
				SuccessCount:        &successCount,
				FailedCount:         &failureCount,
				FunctionsInvoked:    &functionsInvoked,
				RecordOutputChannel: invocationRecordOutputChannel,
				AnnounceDoneWG:      announceDone,
			}

			announceDone.Add(1)
			testDriver.invokeFunction(metadata)

			switch test.forceFail {
			case true:
				if !(successCount == 0 && failureCount == 1 && functionsInvoked == 1) {
					t.Error("The function somehow managed to execute.")
				}
			case false:
				if !(successCount == 1 && failureCount == 0 && functionsInvoked == 1) {
					t.Error("The function should not have failed.")
				}
			}

			record := <-invocationRecordOutputChannel
			announceDone.Wait()

			if record.Phase != int(metadata.Phase) ||
				record.InvocationID != composeInvocationID(common.MinuteGranularity, 0, 0) {

				t.Error("Invalid invocation record received.")
			}
		})
	}
}
func TestDAGInvocation(t *testing.T) {
	var successCount int64 = 0
	var failureCount int64 = 0
	var functionsToInvoke int = 3
	var functionsInvoked int64
	invocationRecordOutputChannel := make(chan *metric.ExecutionRecord, functionsToInvoke)
	announceDone := &sync.WaitGroup{}

	testDriver := createTestDriver([]int{4}, false)
	address, port := "localhost", 8086
	function := testDriver.Configuration.Functions[0]
	function.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go standard.StartGRPCServer(address, port, standard.TraceFunction, "")
	function.Specification.RuntimeSpecification = []common.RuntimeSpecification{{
		Runtime: 1000,
		Memory:  128,
	}}
	functionList := make([]*common.Function, 3)
	for i := 0; i < len(functionList); i++ {
		functionList[i] = function
	}
	originalBranch := []*list.List{
		func() *list.List {
			l := list.New()
			l.PushBack(&common.Node{Function: functionList[0], Depth: 0})
			l.PushBack(&common.Node{Function: functionList[1], Depth: 1})
			return l
		}(),
	}

	newBranch := []*list.List{
		func() *list.List {
			l := list.New()
			l.PushBack(&common.Node{Function: functionList[2], Depth: 1})
			return l
		}(),
	}

	rootFunction := originalBranch[0]
	rootFunction.Front().Value.(*common.Node).Branches = newBranch
	time.Sleep(2 * time.Second)

	metadata := &InvocationMetadata{
		RootFunction:        rootFunction,
		Phase:               common.ExecutionPhase,
		IatIndex:            0,
		InvocationID:        composeInvocationID(common.MinuteGranularity, 0, 0),
		SuccessCount:        &successCount,
		FailedCount:         &failureCount,
		FunctionsInvoked:    &functionsInvoked,
		RecordOutputChannel: invocationRecordOutputChannel,
		AnnounceDoneWG:      announceDone,
	}

	announceDone.Add(1)
	testDriver.invokeFunction(metadata)
	announceDone.Wait()
	if !(successCount == 3 && failureCount == 0) {
		t.Error("Number of successful and failed invocations not as expected.")
	}
	for i := 0; i < functionsToInvoke; i++ {
		record := <-invocationRecordOutputChannel
		if record.Phase != int(metadata.Phase) ||
			record.InvocationID != composeInvocationID(common.MinuteGranularity, 0, 0) {

			t.Error("Invalid invocation record received.")
		}
	}
}

func TestVSwarmDAGInvocation(t *testing.T) {
	var successCount int64 = 0
	var failureCount int64 = 0
	var functionsToInvoke int = 3
	var functionsInvoked int64
	invocationRecordOutputChannel := make(chan *metric.ExecutionRecord, functionsToInvoke)
	announceDone := &sync.WaitGroup{}

	testDriver := createTestDriver([]int{4}, true)
	address, port := "localhost", 8087
	function := testDriver.Configuration.Functions[0]
	function.Endpoint = fmt.Sprintf("%s:%d", address, port)

	go vswarm.StartVSwarmGRPCServer(address, port)
	function.Specification.RuntimeSpecification = []common.RuntimeSpecification{{
		Runtime: 1000,
		Memory:  128,
	}}
	functionList := make([]*common.Function, 3)
	for i := 0; i < len(functionList); i++ {
		functionList[i] = function
	}
	originalBranch := []*list.List{
		func() *list.List {
			l := list.New()
			l.PushBack(&common.Node{Function: functionList[0], Depth: 0})
			l.PushBack(&common.Node{Function: functionList[1], Depth: 1})
			return l
		}(),
	}

	newBranch := []*list.List{
		func() *list.List {
			l := list.New()
			l.PushBack(&common.Node{Function: functionList[2], Depth: 1})
			return l
		}(),
	}

	rootFunction := originalBranch[0]
	rootFunction.Front().Value.(*common.Node).Branches = newBranch
	time.Sleep(2 * time.Second)

	metadata := &InvocationMetadata{
		RootFunction:        rootFunction,
		Phase:               common.ExecutionPhase,
		IatIndex:            0,
		InvocationID:        composeInvocationID(common.MinuteGranularity, 0, 0),
		SuccessCount:        &successCount,
		FailedCount:         &failureCount,
		FunctionsInvoked:    &functionsInvoked,
		RecordOutputChannel: invocationRecordOutputChannel,
		AnnounceDoneWG:      announceDone,
	}

	announceDone.Add(1)
	testDriver.invokeFunction(metadata)
	announceDone.Wait()
	if !(successCount == 3 && failureCount == 0) {
		t.Error("Number of successful and failed invocations not as expected.")
	}
	for i := 0; i < functionsToInvoke; i++ {
		record := <-invocationRecordOutputChannel
		if record.Phase != int(metadata.Phase) ||
			record.InvocationID != composeInvocationID(common.MinuteGranularity, 0, 0) {

			t.Error("Invalid invocation record received.")
		}
	}
}

func TestGlobalMetricsCollector(t *testing.T) {
	driver := createTestDriver([]int{5}, false)

	inputChannel := make(chan *metric.ExecutionRecord)
	totalIssuedChannel := make(chan int64)
	collectorReady, collectorFinished := &sync.WaitGroup{}, &sync.WaitGroup{}

	collectorReady.Add(1)
	collectorFinished.Add(1)

	go metric.CreateGlobalMetricsCollector(driver.outputFilename("duration"), inputChannel, collectorReady, collectorFinished, totalIssuedChannel)
	collectorReady.Wait()

	bogusRecord := &metric.ExecutionRecord{
		ExecutionRecordBase: metric.ExecutionRecordBase{
			Phase:        int(common.ExecutionPhase),
			Instance:     "",
			InvocationID: "min1.inv1",
			StartTime:    123456789,

			RequestedDuration: 1,
			ResponseTime:      2,
			ActualDuration:    3,

			ConnectionTimeout: false,
			FunctionTimeout:   true,
		},
		ActualMemoryUsage: 4,
	}

	for i := 0; i < driver.Configuration.Functions[0].InvocationStats.Invocations[0]; i++ {
		inputChannel <- bogusRecord
	}

	totalIssuedChannel <- int64(driver.Configuration.Functions[0].InvocationStats.Invocations[0])
	collectorFinished.Wait()

	f, err := os.Open(driver.outputFilename("duration"))
	if err != nil {
		t.Error(err)
	}

	var record []metric.ExecutionRecord
	err = gocsv.UnmarshalFile(f, &record)
	if err != nil {
		log.Fatal(err.Error())
	}

	for i := 0; i < driver.Configuration.Functions[0].InvocationStats.Invocations[0]; i++ {
		if record[i] != *bogusRecord {
			t.Error("Failed due to unexpected data received.")
		}
	}
}

func TestDriverBackgroundProcesses(t *testing.T) {
	tests := []struct {
		testName                 string
		metricsCollectionEnabled bool
	}{
		{
			testName:                 "without_metrics",
			metricsCollectionEnabled: false,
		},
		{
			testName:                 "with_metrics",
			metricsCollectionEnabled: true,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			if test.metricsCollectionEnabled {
				// TODO: implement testing once metrics collection feature is ready
				t.Skip("Not yet implemented")
			}

			driver := createTestDriver([]int{5}, false)
			globalCollectorAnnounceDone := &sync.WaitGroup{}

			completed, _, _, _ := driver.startBackgroundProcesses(globalCollectorAnnounceDone)

			completed.Wait()
		})
	}
}

func TestDriverCompletely(t *testing.T) {
	tests := []struct {
		testName              string
		experimentDurationMin int
		withWarmup            bool
		traceGranularity      common.TraceGranularity
		invocationStats       []int
		expectedInvocations   int
	}{
		{
			testName:              "no_invocations",
			experimentDurationMin: 1,
			invocationStats:       []int{0},
			traceGranularity:      common.MinuteGranularity,
			expectedInvocations:   0,
		},
		{
			testName:              "without_warmup",
			experimentDurationMin: 1,
			invocationStats:       []int{5},
			traceGranularity:      common.MinuteGranularity,
			expectedInvocations:   5,
		},
		{
			testName:              "with_warmup",
			experimentDurationMin: 2, // 1 withWarmup + 1 execution
			invocationStats:       []int{5, 5},
			traceGranularity:      common.MinuteGranularity,
			withWarmup:            true,
			expectedInvocations:   10,
		},
		{
			testName:              "without_warmup_second_granularity",
			experimentDurationMin: 1,
			invocationStats: []int{
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
			},
			traceGranularity:    common.SecondGranularity,
			expectedInvocations: 60,
		},
		{
			testName:              "with_warmup_second_granularity",
			experimentDurationMin: 2,
			invocationStats: []int{
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
			},
			traceGranularity:    common.SecondGranularity,
			withWarmup:          true,
			expectedInvocations: 120,
		},
		{
			testName:              "without_warmup_sleep_1min_then_invoke",
			experimentDurationMin: 2,
			invocationStats:       []int{0, 5},
			expectedInvocations:   5,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			logrus.SetLevel(logrus.DebugLevel)
			logrus.SetFormatter(&logrus.TextFormatter{TimestampFormat: time.StampMilli, FullTimestamp: true})

			driver := createTestDriver(test.invocationStats, false)

			if test.withWarmup {
				if test.traceGranularity == common.MinuteGranularity {
					driver.Configuration.LoaderConfiguration.WarmupDuration = 1
				} else {
					driver.Configuration.LoaderConfiguration.WarmupDuration = 60
				}
			}
			driver.Configuration.TraceDuration = test.experimentDurationMin
			driver.Configuration.TraceGranularity = test.traceGranularity

			driver.GenerateSpecification()
			driver.RunExperiment()

			f, err := os.Open(driver.outputFilename("duration"))
			if err != nil {
				t.Error(err)
			}

			if test.expectedInvocations == 0 {
				return
			}

			var records []metric.ExecutionRecordBase
			err = gocsv.UnmarshalFile(f, &records)
			if err != nil {
				log.Fatal(err.Error())
			}

			successfulInvocation, failedInvocations := 0, 0
			//clockTolerance := int64(20_000) // ms

			for i := 0; i < len(records); i++ {
				record := records[i]

				if test.withWarmup {
					threshold := 60
					if test.testName == "with_warmup" {
						threshold = 5
					}

					// 60 no checked since it is started in the warmup phase and completed in the execution phase -- new value taken
					if i < threshold && record.Phase != int(common.WarmupPhase) {
						t.Error("Invalid record phase in warmup.")
					} else if i > threshold && record.Phase != int(common.ExecutionPhase) {
						t.Errorf("Invalid record phase in execution phase - ID = %d.", i)
					}
				}

				if !record.ConnectionTimeout && !record.FunctionTimeout {
					successfulInvocation++
				} else {
					failedInvocations++
				}

				/*if i < len(records)-1 {
					diff := (records[i+1].StartTime - records[i].StartTime) / 1_000_000 // ms

					if diff > clockTolerance {
						t.Errorf("Too big clock drift for the test to pass - %d.", diff)
					}
				}*/
			}

			expectedInvocations := test.expectedInvocations
			if !(successfulInvocation == expectedInvocations && failedInvocations == 0) {
				t.Error("Number of successful and failed invocations do not match.")
			}
		})
	}
}

func TestVSwarmDriverCompletely(t *testing.T) {
	tests := []struct {
		testName              string
		experimentDurationMin int
		withWarmup            bool
		traceGranularity      common.TraceGranularity
		invocationStats       []int
		expectedInvocations   int
	}{
		{
			testName:              "without_warmup",
			experimentDurationMin: 1,
			invocationStats:       []int{5},
			traceGranularity:      common.MinuteGranularity,
			expectedInvocations:   5,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			logrus.SetLevel(logrus.DebugLevel)
			logrus.SetFormatter(&logrus.TextFormatter{TimestampFormat: time.StampMilli, FullTimestamp: true})

			driver := createTestDriver(test.invocationStats, true)

			if test.withWarmup {
				if test.traceGranularity == common.MinuteGranularity {
					driver.Configuration.LoaderConfiguration.WarmupDuration = 1
				} else {
					driver.Configuration.LoaderConfiguration.WarmupDuration = 60
				}
			}
			driver.Configuration.TraceDuration = test.experimentDurationMin
			driver.Configuration.TraceGranularity = test.traceGranularity

			driver.GenerateSpecification()
			driver.RunExperiment()

			f, err := os.Open(driver.outputFilename("duration"))
			if err != nil {
				t.Error(err)
			}

			if test.expectedInvocations == 0 {
				return
			}

			var records []metric.ExecutionRecordBase
			err = gocsv.UnmarshalFile(f, &records)
			if err != nil {
				log.Fatal(err.Error())
			}

			successfulInvocation, failedInvocations := 0, 0
			//clockTolerance := int64(20_000) // ms

			for i := 0; i < len(records); i++ {
				record := records[i]

				if test.withWarmup {
					threshold := 60
					if test.testName == "with_warmup" {
						threshold = 5
					}

					// 60 no checked since it is started in the warmup phase and completed in the execution phase -- new value taken
					if i < threshold && record.Phase != int(common.WarmupPhase) {
						t.Error("Invalid record phase in warmup.")
					} else if i > threshold && record.Phase != int(common.ExecutionPhase) {
						t.Errorf("Invalid record phase in execution phase - ID = %d.", i)
					}
				}

				if !record.ConnectionTimeout && !record.FunctionTimeout {
					successfulInvocation++
				} else {
					failedInvocations++
				}

				/*if i < len(records)-1 {
					diff := (records[i+1].StartTime - records[i].StartTime) / 1_000_000 // ms

					if diff > clockTolerance {
						t.Errorf("Too big clock drift for the test to pass - %d.", diff)
					}
				}*/
			}

			expectedInvocations := test.expectedInvocations
			if !(successfulInvocation == expectedInvocations && failedInvocations == 0) {
				t.Error("Number of successful and failed invocations do not match.")
			}
		})
	}
}
func TestHasMinuteExpired(t *testing.T) {
	if !hasMinuteExpired(time.Now().Add(-2 * time.Minute)) {
		t.Error("Time should have expired.")
	}

	if hasMinuteExpired(time.Now().Add(-30 * time.Second)) {
		t.Error("Time shouldn't have expired.")
	}
}

func TestRequestedVsIssued(t *testing.T) {
	if !isRequestTargetAchieved(100, 100*(1-common.RequestedVsIssuedWarnThreshold+0.05), common.RequestedVsIssued) {
		t.Error("Unexpected value received.")
	}

	if !isRequestTargetAchieved(100, 100*(1-common.RequestedVsIssuedWarnThreshold-0.05), common.RequestedVsIssued) {
		t.Error("Unexpected value received.")
	}

	if isRequestTargetAchieved(100, 100*(1-common.RequestedVsIssuedWarnThreshold-0.15), common.RequestedVsIssued) {
		t.Error("Unexpected value received.")
	}

	if isRequestTargetAchieved(100, 100*(common.FailedWarnThreshold-0.1), common.IssuedVsFailed) {
		t.Error("Unexpected value received.")
	}

	if isRequestTargetAchieved(100, 100*(common.FailedWarnThreshold+0.05), common.IssuedVsFailed) {
		t.Error("Unexpected value received.")
	}

	if isRequestTargetAchieved(100, 100*(common.FailedTerminateThreshold-0.1), common.IssuedVsFailed) {
		t.Error("Unexpected value received.")
	}
}
