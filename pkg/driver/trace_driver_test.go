package driver

import (
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/metric"
	"github.com/eth-easl/loader/pkg/workload/standard"
	"github.com/gocarina/gocsv"
	"os"
	"sync"
	"testing"
	"time"
)

func createTestDriver() *Driver {
	driver := NewDriver(&DriverConfiguration{
		EnableMetricsCollection: false,
		IATDistribution:         common.Equidistant,

		Functions: []*common.Function{
			{
				Name:                    "test-function",
				NumInvocationsPerMinute: []int{5},
				RuntimeStats: common.FunctionRuntimeStats{
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
				MemoryStats: common.FunctionMemoryStats{
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
		TotalNumInvocationsEachMinute: []int{5},
		WithTracing:                   false,
		Seed:                          123456789,
		TestMode:                      true,
	})

	driver.OutputFilename = "../../data/out/trace_driver_test.csv"

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
			port:      8080,
			forceFail: true,
		},
		{
			testName:  "invoke_success",
			port:      8081,
			forceFail: false,
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			var successCount int64 = 0
			var failureCount int64 = 0

			invocationRecordOutputChannel := make(chan *metric.ExecutionRecord, 1)
			announceDone := &sync.WaitGroup{}

			testDriver := createTestDriver()

			if !test.forceFail {
				address, port := "localhost", test.port
				testDriver.Configuration.Functions[0].Endpoint = fmt.Sprintf("%s:%d", address, port)

				go standard.StartGRPCServer(address, port, "")

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
				MinuteIndex:         1,
				InvocationIndex:     2,
				SuccessCount:        &successCount,
				FailedCount:         &failureCount,
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

			record := <-invocationRecordOutputChannel
			announceDone.Wait()

			if record.Phase != int(metadata.Phase) ||
				record.InvocationID != composeInvocationID(metadata.MinuteIndex, metadata.InvocationIndex) {

				t.Error("Invalid invocation record received.")
			}
		})
	}
}

func TestGlobalMetricsCollector(t *testing.T) {
	driver := createTestDriver()

	inputChannel := make(chan *metric.ExecutionRecord)
	collectorReady, collectorFinished := &sync.WaitGroup{}, &sync.WaitGroup{}

	collectorReady.Add(1)
	collectorFinished.Add(1)

	go driver.createGlobalMetricsCollector(driver.OutputFilename, inputChannel, collectorReady, collectorFinished)
	collectorReady.Wait()

	bogusRecord := &metric.ExecutionRecord{
		Phase:        common.ExecutionPhase,
		FunctionName: driver.Configuration.Functions[0].Name,
		InvocationID: "min1.inv1",
		StartTime:    123456789,

		RequestedDuration: 1,
		ResponseTime:      2,
		ActualDuration:    3,
		ActualMemoryUsage: 4,

		ConnectionTimeout: false,
		FunctionTimeout:   true,
	}

	for i := 0; i < driver.Configuration.TotalNumInvocationsEachMinute[0]; i++ {
		inputChannel <- bogusRecord
	}

	collectorFinished.Wait()

	f, err := os.Open(driver.OutputFilename)
	if err != nil {
		t.Error(err)
	}

	var record []metric.ExecutionRecord
	gocsv.UnmarshalFile(f, &record)

	for i := 0; i < driver.Configuration.TotalNumInvocationsEachMinute[0]; i++ {
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

			completed, _ := driver.startBackgroundProcesses(globalCollectorAnnounceDone)

			completed.Wait()
		})
	}
}

func TestDriverCompletely(t *testing.T) {
	driver := createTestDriver()
	driver.RunExperiment()

	f, err := os.Open(driver.OutputFilename)
	if err != nil {
		t.Error(err)
	}

	var records []metric.ExecutionRecord
	gocsv.UnmarshalFile(f, &records)

	successfulInvocation, failedInvocations := 0, 0
	clockTolerance := int64(20_000) // ms

	for i := 0; i < len(records); i++ {
		record := records[i]

		if !record.ConnectionTimeout && !record.FunctionTimeout {
			successfulInvocation++
		} else {
			failedInvocations++
		}

		if i < len(records)-1 {
			diff := (records[i+1].StartTime - records[i].StartTime) / 1_0000_000 // ms

			if diff > clockTolerance {
				t.Error("Too big clock drift for the test to pass.")
			}
		}
	}

	if !(successfulInvocation == 5 && failedInvocations == 0) {
		t.Error("Number of successful and failed invocations do not match.")
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
	if !isRequestTargetAchieved(100, 95) {
		t.Error("Unexpected value received.")
	}

	if !isRequestTargetAchieved(100, 85) {
		t.Error("Unexpected value received.")
	}

	if isRequestTargetAchieved(100, 75) {
		t.Error("Unexpected value received.")
	}
}
