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
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/metric"
	"github.com/vhive-serverless/loader/pkg/workload/standard"
)

func createTestDriver() *Driver {
	cfg := createFakeLoaderConfiguration()

	driver := NewDriver(&DriverConfiguration{
		LoaderConfiguration: cfg,
		IATDistribution:     common.Equidistant,
		TraceDuration:       1,

		Functions: []*common.Function{
			{
				Name: "test-function",
				InvocationStats: &common.FunctionInvocationStats{
					Invocations: []int{
						5, 5, 5, 5, 5,
						5, 5, 5, 5, 5,
						5, 5, 5, 5, 5,
						5, 5, 5, 5, 5,
					},
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

			invocationRecordOutputChannel := make(chan interface{}, 1)
			announceDone := &sync.WaitGroup{}

			testDriver := createTestDriver()
			var failureCountByMinute = make([]int64, testDriver.Configuration.TraceDuration)

			if !test.forceFail {
				address, port := "localhost", test.port
				testDriver.Configuration.Functions[0].Endpoint = fmt.Sprintf("%s:%d", address, port)

				go standard.StartGRPCServer(address, port, standard.TraceFunction, "")

				// make sure that the gRPC server is running
				time.Sleep(2 * time.Second)
			}

			metadata := &InvocationMetadata{
				Function: testDriver.Configuration.Functions[0],
				RuntimeSpecifications: &common.RuntimeSpecification{
					Runtime: 1000,
					Memory:  128,
				},
				Phase:               common.ExecutionPhase,
				MinuteIndex:         0,
				InvocationIndex:     2,
				SuccessCount:        &successCount,
				FailedCount:         &failureCount,
				FailedCountByMinute: failureCountByMinute,
				RecordOutputChannel: invocationRecordOutputChannel,
				AnnounceDoneWG:      announceDone,
			}

			announceDone.Add(1)
			testDriver.invokeFunction(metadata)

			switch test.forceFail {
			case true:
				if !(successCount == 0 && failureCount == 1) {
					t.Error("The function somehow managed to execute.")
				}
			case false:
				if !(successCount == 1 && failureCount == 0) {
					t.Error("The function should not have failed.")
				}
			}

			record := (<-invocationRecordOutputChannel).(*metric.ExecutionRecord)
			announceDone.Wait()

			if record.Phase != int(metadata.Phase) ||
				record.InvocationID != composeInvocationID(common.MinuteGranularity, metadata.MinuteIndex, metadata.InvocationIndex) {

				t.Error("Invalid invocation record received.")
			}
		})
	}
}

func TestGlobalMetricsCollector(t *testing.T) {
	driver := createTestDriver()

	inputChannel := make(chan interface{})
	totalIssuedChannel := make(chan int64)
	collectorReady, collectorFinished := &sync.WaitGroup{}, &sync.WaitGroup{}

	collectorReady.Add(1)
	collectorFinished.Add(1)

	go driver.createGlobalMetricsCollector(driver.outputFilename("duration"), inputChannel, collectorReady, collectorFinished, totalIssuedChannel)
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
		log.Fatalf(err.Error())
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

			driver := createTestDriver()
			globalCollectorAnnounceDone := &sync.WaitGroup{}

			completed, _, _, _ := driver.startBackgroundProcesses(globalCollectorAnnounceDone)

			completed.Wait()
		})
	}
}

func TestDriverCompletely(t *testing.T) {
	tests := []struct {
		testName          string
		withWarmup        bool
		secondGranularity bool
	}{
		{
			testName:   "without_warmup",
			withWarmup: false,
		},
		{
			testName:   "with_warmup",
			withWarmup: true,
		},
		{
			testName:          "without_warmup_second_granularity",
			withWarmup:        false,
			secondGranularity: true,
		},
		{
			testName:          "with_warmup_second_granularity",
			withWarmup:        true,
			secondGranularity: true,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			logrus.SetLevel(logrus.DebugLevel)

			driver := createTestDriver()
			if test.withWarmup {
				driver.Configuration.LoaderConfiguration.WarmupDuration = 1
				driver.Configuration.TraceDuration = 3 // 1 profiling - 1 withWarmup - 1 execution
			}
			if test.secondGranularity {
				driver.Configuration.TraceGranularity = common.SecondGranularity
			}

			driver.RunExperiment(false, false)

			f, err := os.Open(driver.outputFilename("duration"))
			if err != nil {
				t.Error(err)
			}

			var records []metric.ExecutionRecordBase
			err = gocsv.UnmarshalFile(f, &records)
			if err != nil {
				log.Fatalf(err.Error())
			}

			successfulInvocation, failedInvocations := 0, 0
			clockTolerance := int64(20_000) // ms

			for i := 0; i < len(records); i++ {
				record := records[i]

				if test.withWarmup {
					if i < 5 && record.Phase != int(common.WarmupPhase) {
						t.Error("Invalid record phase in warmup.")
					} else if i > 5 && record.Phase != int(common.ExecutionPhase) {
						t.Error("Invalid record phase in execution phase.")
					}
				}

				if !record.ConnectionTimeout && !record.FunctionTimeout {
					successfulInvocation++
				} else {
					failedInvocations++
				}

				if i < len(records)-1 {
					diff := (records[i+1].StartTime - records[i].StartTime) / 1_000_000 // ms

					if diff > clockTolerance {
						t.Error("Too big clock drift for the test to pass.")
					}
				}
			}

			expectedInvocations := 5
			if test.withWarmup {
				expectedInvocations = 10
			}

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

func TestProceedToNextMinute(t *testing.T) {
	function := &common.Function{
		Name: "test-function",
		InvocationStats: &common.FunctionInvocationStats{
			Invocations: []int{100, 100, 100, 100, 100},
		},
	}

	tests := []struct {
		testName        string
		minuteIndex     int
		invocationIndex int
		failedCount     int64
		skipMinute      bool
		toBreak         bool
	}{
		{
			testName:        "proceed_to_next_minute_no_break_no_fail",
			minuteIndex:     0,
			invocationIndex: 95,
			failedCount:     0,
			skipMinute:      false,
			toBreak:         false,
		},
		{
			testName:        "proceed_to_next_minute_break_no_fail",
			minuteIndex:     0,
			invocationIndex: 75,
			failedCount:     0,
			skipMinute:      false,
			toBreak:         true,
		},
		{
			testName:        "proceed_to_next_minute_break_with_fail",
			minuteIndex:     0,
			invocationIndex: 90,
			failedCount:     55,
			skipMinute:      false,
			toBreak:         true,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			driver := createTestDriver()

			minuteIndex := test.minuteIndex
			invocationIndex := test.invocationIndex
			startOfMinute := time.Now()
			phase := common.ExecutionPhase
			var failedCountByMinute = make([]int64, driver.Configuration.TraceDuration)
			failedCountByMinute[minuteIndex] = test.failedCount
			var iatSum int64 = 2500

			toBreak := driver.proceedToNextMinute(function, &minuteIndex, &invocationIndex, &startOfMinute,
				test.skipMinute, &phase, failedCountByMinute, &iatSum)

			if toBreak != test.toBreak {
				t.Error("Invalid response from minute cleanup procedure.")
			}

			if !toBreak && ((minuteIndex != test.minuteIndex+1) || (invocationIndex != 0) || (failedCountByMinute[test.minuteIndex] != 0) || (iatSum != 0)) {
				t.Error("Invalid response from minute cleanup procedure.")
			}
		})
	}
}
