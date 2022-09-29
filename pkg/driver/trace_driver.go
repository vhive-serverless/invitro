package driver

import (
	"fmt"
	util "github.com/eth-easl/loader/pkg"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/generator"
	log "github.com/sirupsen/logrus"
	"os"
	"sync"
	"sync/atomic"
	"time"

	mc "github.com/eth-easl/loader/pkg/metric"
)

type DriverConfiguration struct {
	EnableMetricsCollection bool
	IATDistribution         common.IatDistribution

	Functions                     []common.Function
	TotalNumInvocationsEachMinute []int
	WithTracing                   bool
	Seed                          int64
}

type Driver struct {
	collector            mc.Collector
	clusterUsage         mc.ClusterUsage
	knStats              mc.KnStats
	coldStartGauge       int
	coldStartMinuteCount int // TODO: maybe set to -1 if scraping is not enabled

	Configuration          *DriverConfiguration
	SpecificationGenerator *generator.SpecificationGenerator
}

func NewDriver(driverConfig *DriverConfiguration) *Driver {
	return &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.Seed),
	}
}

/////////////////////////////////////////
// METRICS SCRAPPERS
/////////////////////////////////////////

// CreateKnativeMetricsScrapper launches a scraper that updates the cluster usage periodically
func (d *Driver) CreateKnativeMetricsScrapper(interval time.Duration, signalReady *sync.WaitGroup) func() {
	timer := time.NewTicker(interval)
	d.collector = mc.NewCollector()

	return func() {
		signalReady.Done()

		for {
			<-timer.C
			d.clusterUsage = mc.ScrapeClusterUsage()
		}
	}
}

// CreateColdStartCountScrapper creates cold start count scrapper with the given period
func (d *Driver) CreateColdStartCountScrapper(interval time.Duration, signalReady *sync.WaitGroup) func() {
	timer := time.NewTicker(time.Second * 60)
	d.knStats = mc.KnStats{}
	d.coldStartGauge = 0
	d.coldStartMinuteCount = 0

	return func() {
		signalReady.Done()

		for {
			<-timer.C
			d.coldStartGauge = d.collector.RecordScalesAndGetColdStartCount()
			d.coldStartMinuteCount += d.coldStartGauge
		}
	}
}

// CreateKnativeStateUpdateScrapper creates a scraper that updates Knative states periodically
func (d *Driver) CreateKnativeStateUpdateScrapper(interval time.Duration, signalReady *sync.WaitGroup) func() {
	timer := time.NewTicker(interval)
	d.clusterUsage = mc.ClusterUsage{}

	return func() {
		signalReady.Done()

		for {
			<-timer.C
			d.knStats = mc.ScrapeKnStats()
		}
	}
}

/////////////////////////////////////////
// DRIVER LOGIC
/////////////////////////////////////////

type InvocationMetadata struct {
	Function              common.Function
	RuntimeSpecifications *common.RuntimeSpecification
	Phase                 common.ExperimentPhase

	MinuteIndex     int
	InvocationIndex int

	SuccessCount *int64
	FailedCount  *int64

	RecordOutputChannel chan *mc.ExecutionRecord
	AnnounceDoneWG      *sync.WaitGroup
}

func (d *Driver) invokeFunction(metadata *InvocationMetadata) {
	defer metadata.AnnounceDoneWG.Done()

	success, record := Invoke(metadata.Function, metadata.RuntimeSpecifications, d.Configuration.WithTracing)

	record.Phase = int(metadata.Phase)
	record.InvocationID = fmt.Sprintf("min%d.inv%d", metadata.MinuteIndex, metadata.InvocationIndex)

	if success {
		atomic.AddInt64(metadata.SuccessCount, 1)
	} else {
		atomic.AddInt64(metadata.FailedCount, 1)
	}

	metadata.RecordOutputChannel <- record
}

func (d *Driver) individualFunctionDriver(function common.Function, announceFunctionDone *sync.WaitGroup,
	totalSuccessfull *int64, totalFailed *int64, recordOutputChannel chan *mc.ExecutionRecord) {

	totalTraceDuration := len(d.Configuration.TotalNumInvocationsEachMinute)
	minuteIndex, invocationIndex := 0, 0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	var successfullInvocations int64
	var failedInvocations int64

	waitForInvocations := sync.WaitGroup{}

	for {
		// Check whether the end of trace has been reached
		if minuteIndex >= totalTraceDuration {
			break
		}

		if function.NumInvocationsPerMinute[minuteIndex] == 0 {
			minuteIndex++
			invocationIndex = 0

			time.Sleep(time.Minute)
		} else {
			waitForInvocations.Add(1)

			go d.invokeFunction(&InvocationMetadata{
				Function:              function,
				RuntimeSpecifications: &runtimeSpecification[minuteIndex][invocationIndex],
				Phase:                 common.ExecutionPhase, // TODO: add a warmup phase
				MinuteIndex:           minuteIndex,
				InvocationIndex:       invocationIndex,
				SuccessCount:          &successfullInvocations,
				FailedCount:           &failedInvocations,
				RecordOutputChannel:   recordOutputChannel,
				AnnounceDoneWG:        &waitForInvocations,
			})

			sleepFor := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond

			invocationIndex++
			if function.NumInvocationsPerMinute[minuteIndex] == invocationIndex {
				minuteIndex++
				invocationIndex = 0

				if minuteIndex >= totalTraceDuration {
					break
				}
			}

			//fmt.Println(sleepFor)
			time.Sleep(sleepFor)
		}
	}

	waitForInvocations.Wait()

	log.Infof("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessfull, successfullInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
}

func (d *Driver) globalTimekeeper(totalTraceDuration int, signalReady *sync.WaitGroup) {
	ticker := time.NewTicker(time.Minute)
	globalTimeCounter := 0

	signalReady.Done()

	for {
		<-ticker.C

		log.Infof("End of minute %d\n", globalTimeCounter)

		globalTimeCounter++
		if globalTimeCounter >= totalTraceDuration {
			break
		}

		log.Infof("Start of minute %d\n", globalTimeCounter)
	}

	ticker.Stop()
}

func (d *Driver) createGlobalMetricsCollector(collector chan *mc.ExecutionRecord, signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup) {
	totalNumberOfInvocations := SumIntArray(d.Configuration.TotalNumInvocationsEachMinute)
	currentlyWritten := 0

	// TODO: will be changed afterward
	invocationFile, err := os.Create("data/out/test_output.csv")
	util.Check(err)
	defer invocationFile.Close()

	invocationFile.WriteString(
		"phase," +
			"functionName," +
			"invocationID," +
			"startTime," +
			"requestedDuration," +
			"responseTime," +
			"actualDuration," +
			"actualMemoryUsage," +
			"connectionTimeout," +
			"functionTimeout\n")

	signalReady.Done()

	for {
		select {
		case record := <-collector:
			invocationFile.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d,%d,%d,%d,%t,%t\n",
				record.Phase,
				record.FunctionName,
				record.InvocationID,
				record.StartTime,
				record.RequestedDuration,
				record.ResponseTime,
				record.ActualDuration,
				record.ActualMemoryUsage,
				record.ConnectionTimeout,
				record.FunctionTimeout,
			))

			currentlyWritten++
			if currentlyWritten == totalNumberOfInvocations {
				(*signalEverythingWritten).Done()

				break
			}
		}
	}
}

func (d *Driver) startBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (*sync.WaitGroup, chan *mc.ExecutionRecord) {
	auxiliaryProcessBarrier := &sync.WaitGroup{}

	if d.Configuration.EnableMetricsCollection {
		// TODO: these following arguments should be configurable
		auxiliaryProcessBarrier.Add(3)

		// TODO: the following three go routines are untested
		go d.CreateKnativeMetricsScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateKnativeStateUpdateScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateColdStartCountScrapper(time.Second*60, auxiliaryProcessBarrier)
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan *mc.ExecutionRecord)
	go d.createGlobalMetricsCollector(globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten)

	// TODO: need assert on trace parsing that the last column with non null is parsed and declared as trace length
	traceDurationInMinutes := len(d.Configuration.TotalNumInvocationsEachMinute)
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector
}

func SumIntArray(x []int) int {
	result := 0

	for i := 0; i < len(x); i++ {
		result += x[i]
	}

	return result
}

func (d *Driver) RunExperiment() int {
	var successfullInvocations int64
	var failedInvocations int64

	allFunctionsCompleted := sync.WaitGroup{}
	allRecordsWritten := sync.WaitGroup{}
	allRecordsWritten.Add(1)

	backgroundProcessesInitializationBarrier, globalMetricsCollector := d.startBackgroundProcesses(&allRecordsWritten)

	log.Infof("Generating IAT and runtime specifications for all the functions\n")
	for i, function := range d.Configuration.Functions {
		spec := d.SpecificationGenerator.GenerateInvocationData(
			function,
			d.Configuration.IATDistribution,
		)

		d.Configuration.Functions[i].Specification = spec
	}

	backgroundProcessesInitializationBarrier.Wait()

	log.Infof("Starting function invocation driver\n")
	for _, function := range d.Configuration.Functions {
		allFunctionsCompleted.Add(1)

		go d.individualFunctionDriver(
			function,
			&allFunctionsCompleted,
			&successfullInvocations,
			&failedInvocations,
			globalMetricsCollector,
		)
	}

	allFunctionsCompleted.Wait()
	allRecordsWritten.Wait()

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d\n", successfullInvocations)
	log.Infof("Number of failed invocations: \t%d\n", failedInvocations)

	return 0
}

/////////////////////////////////////
// TODO: check and refactor everything below
/////////////////////////////////////

/**
 * This function waits for the waitgroup for the specified max timeout.
 * Returns true if waiting timed out.
 */
/*func wgWaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	log.Info("Start waiting for all requests to return.")
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		log.Info("Finished waiting for all invocations.")
		return false
	case <-time.After(timeout):
		return true
	}
}*/

/*func CheckOverload(successCount, failureCount int64) bool {
	//* Amongst those returned, how many has failed?
	failureRate := float64(failureCount) / float64(successCount+failureCount)
	log.Info("Failure rate=", failureRate)
	return failureRate > common.OVERFLOAD_THRESHOLD
}

func DumpOverloadFlag() {
	// If the file doesn't exist, create it, or append to the file
	_, err := os.OpenFile("overload.flag", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
}*/
