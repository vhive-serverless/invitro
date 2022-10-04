package driver

import (
	"fmt"
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

	Functions                     []*common.Function
	TotalNumInvocationsEachMinute []int // TODO: deprecate field
	WithTracing                   bool
	Seed                          int64
	TestMode                      bool
}

type Driver struct {
	collector            mc.Collector
	clusterUsage         mc.ClusterUsage
	knStats              mc.KnStats
	coldStartGauge       int
	coldStartMinuteCount int // TODO: maybe set to -1 if scraping is not enabled

	Configuration          *DriverConfiguration
	SpecificationGenerator *generator.SpecificationGenerator
	OutputFilename         string
}

func NewDriver(driverConfig *DriverConfiguration) *Driver {
	return &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.Seed),
		OutputFilename:         fmt.Sprintf("data/out/exec_duration_%d.csv", len(driverConfig.TotalNumInvocationsEachMinute)),
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
	Function              *common.Function
	RuntimeSpecifications *common.RuntimeSpecification
	Phase                 common.ExperimentPhase

	MinuteIndex     int
	InvocationIndex int

	SuccessCount *int64
	FailedCount  *int64

	RecordOutputChannel chan *mc.ExecutionRecord
	AnnounceDoneWG      *sync.WaitGroup
}

func composeInvocationID(minuteIndex int, invocationIndex int) string {
	return fmt.Sprintf("min%d.inv%d", minuteIndex, invocationIndex)
}

func (d *Driver) invokeFunction(metadata *InvocationMetadata) {
	defer metadata.AnnounceDoneWG.Done()

	success, record := Invoke(metadata.Function, metadata.RuntimeSpecifications, d.Configuration.WithTracing)

	record.Phase = int(metadata.Phase)
	record.InvocationID = composeInvocationID(metadata.MinuteIndex, metadata.InvocationIndex)

	if success {
		atomic.AddInt64(metadata.SuccessCount, 1)
	} else {
		atomic.AddInt64(metadata.FailedCount, 1)
	}

	metadata.RecordOutputChannel <- record
}

func (d *Driver) individualFunctionDriver(function *common.Function, announceFunctionDone *sync.WaitGroup,
	totalSuccessfull *int64, totalFailed *int64, recordOutputChannel chan *mc.ExecutionRecord) {

	totalTraceDuration := len(d.Configuration.TotalNumInvocationsEachMinute)
	minuteIndex, invocationIndex := 0, 0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	var successfullInvocations int64
	var failedInvocations int64

	waitForInvocations := sync.WaitGroup{}

	startOfMinute := time.Now()
	for {
		if minuteIndex >= totalTraceDuration {
			// Check whether the end of trace has been reached
			break
		} else if function.NumInvocationsPerMinute[minuteIndex] == 0 {
			// Sleep for a minute if there are no invocations
			prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, true)

			time.Sleep(time.Minute)
			continue
		}

		if !d.Configuration.TestMode {
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
		} else {
			// To be used from within the Golang testing framework
			log.Debugf("Bogus invocation fired.\n")

			recordOutputChannel <- &mc.ExecutionRecord{
				StartTime:    time.Now().UnixNano(),
				InvocationID: composeInvocationID(minuteIndex, invocationIndex),
			}

			successfullInvocations++
		}

		sleepFor := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond
		time.Sleep(sleepFor)

		invocationIndex++
		if function.NumInvocationsPerMinute[minuteIndex] == invocationIndex {
			prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, false)
		} else if hasMinuteExpired(startOfMinute) {
			if !isRequestTargetAchieved(function.NumInvocationsPerMinute[minuteIndex], invocationIndex) {
				// Not fatal because we want to keep the measurements to be written to the output file
				log.Infof("Requested vs. issued invocations divergence is greater than 20%%. Terminating experiment!\n")

				break
			}

			prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, false)
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessfull, successfullInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
}

func prepareForNextMinute(minuteIndex *int, invocationIndex *int, startOfMinute *time.Time, skipMinute bool) {
	*minuteIndex++
	*invocationIndex = 0
	if !skipMinute {
		*startOfMinute = time.Now()
	} else {
		*startOfMinute = time.Now().Add(time.Minute)
	}
}

func isRequestTargetAchieved(requested int, issued int) bool {
	ratio := float64(requested-issued) / float64(requested)

	if ratio < 0 || ratio > 1 {
		log.Fatalf("Invalid requsted/issued arguments.\n")
	} else if ratio >= 0.2 {
		return false
	}

	if ratio >= 0.1 && ratio < 0.2 {
		log.Warnf("Requested vs. issued invocations divergence is %.2f.\n", ratio)
	}

	return true
}

func hasMinuteExpired(t1 time.Time) bool {
	if time.Now().Sub(t1) > time.Minute {
		return true
	} else {
		return false
	}
}

func (d *Driver) globalTimekeeper(totalTraceDuration int, signalReady *sync.WaitGroup) {
	ticker := time.NewTicker(time.Minute)
	globalTimeCounter := 0

	signalReady.Done()

	for {
		<-ticker.C

		log.Debugf("End of minute %d\n", globalTimeCounter)

		globalTimeCounter++
		if globalTimeCounter >= totalTraceDuration {
			break
		}

		log.Debugf("Start of minute %d\n", globalTimeCounter)
	}

	ticker.Stop()
}

func (d *Driver) createGlobalMetricsCollector(filename string, collector chan *mc.ExecutionRecord,
	signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup) {

	totalNumberOfInvocations := common.SumIntArray(d.Configuration.TotalNumInvocationsEachMinute)
	currentlyWritten := 0

	invocationFile, err := os.Create(filename)
	common.Check(err)
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
		auxiliaryProcessBarrier.Add(3)

		// TODO: the following three go routines are untested
		// TODO: the following arguments should be configurable
		go d.CreateKnativeMetricsScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateKnativeStateUpdateScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateColdStartCountScrapper(time.Second*60, auxiliaryProcessBarrier)
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan *mc.ExecutionRecord)
	go d.createGlobalMetricsCollector(d.OutputFilename, globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten)

	// TODO: need assert on trace parsing that the last column with non null is parsed and declared as trace length
	traceDurationInMinutes := len(d.Configuration.TotalNumInvocationsEachMinute)
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector
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

	// TODO: warmup phase comes in here

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
