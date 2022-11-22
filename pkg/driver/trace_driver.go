package driver

import (
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/config"
	"github.com/eth-easl/loader/pkg/generator"
	"github.com/eth-easl/loader/pkg/trace"
	log "github.com/sirupsen/logrus"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	mc "github.com/eth-easl/loader/pkg/metric"
)

type DriverConfiguration struct {
	LoaderConfiguration *config.LoaderConfiguration
	IATDistribution     common.IatDistribution
	TraceDuration       int // in minutes

	YAMLPath string
	TestMode bool

	Functions []*common.Function
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
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.LoaderConfiguration.Seed),
		OutputFilename:         fmt.Sprintf("%s_duration_%d.csv", driverConfig.LoaderConfiguration.OutputPathPrefix, driverConfig.TraceDuration),
	}
}

func (c *DriverConfiguration) WithWarmup() bool {
	if c.LoaderConfiguration.WarmupDuration > 0 {
		return true
	} else {
		return false
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

	SuccessCount           *int64
	FailedCount            *int64
	ApproximateFailedCount *int64

	RecordOutputChannel chan *mc.ExecutionRecord
	AnnounceDoneWG      *sync.WaitGroup
}

func composeInvocationID(minuteIndex int, invocationIndex int) string {
	return fmt.Sprintf("min%d.inv%d", minuteIndex, invocationIndex)
}

func (d *Driver) invokeFunction(metadata *InvocationMetadata) {
	defer metadata.AnnounceDoneWG.Done()

	success, record := Invoke(metadata.Function, metadata.RuntimeSpecifications, d.Configuration.LoaderConfiguration)

	record.Phase = int(metadata.Phase)
	record.InvocationID = composeInvocationID(metadata.MinuteIndex, metadata.InvocationIndex)

	if success {
		atomic.AddInt64(metadata.SuccessCount, 1)
	} else {
		atomic.AddInt64(metadata.FailedCount, 1)
		atomic.AddInt64(metadata.ApproximateFailedCount, 1)
	}

	metadata.RecordOutputChannel <- record
}

func (d *Driver) individualFunctionDriver(function *common.Function, announceFunctionDone *sync.WaitGroup,
	totalSuccessful *int64, totalFailed *int64, totalIssued *int64, recordOutputChannel chan *mc.ExecutionRecord) {

	totalTraceDuration := d.Configuration.TraceDuration
	minuteIndex, invocationIndex := 0, 0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	var successfullInvocations int64
	var failedInvocations int64
	var approximateFailedCount int64
	var numberOfIssuedInvocations int64
	var currentPhase common.ExperimentPhase = common.ExecutionPhase

	waitForInvocations := sync.WaitGroup{}

	if d.Configuration.WithWarmup() {
		currentPhase = common.WarmupPhase
		// skip the first minute because of profiling
		minuteIndex = 1

		log.Infof("Warmup phase has started.")
	}

	startOfMinute := time.Now()
	var previousIATSum int64

	for {
		if minuteIndex >= totalTraceDuration {
			// Check whether the end of trace has been reached
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			// Sleep for a minute if there are no invocations
			d.proceedToNextMinute(function, &minuteIndex, &invocationIndex,
				&startOfMinute, true, &currentPhase, &approximateFailedCount, &previousIATSum)

			time.Sleep(time.Minute)

			continue
		}

		numberOfIssuedInvocations++
		if !d.Configuration.TestMode {
			waitForInvocations.Add(1)

			go d.invokeFunction(&InvocationMetadata{
				Function:               function,
				RuntimeSpecifications:  &runtimeSpecification[minuteIndex][invocationIndex],
				Phase:                  currentPhase,
				MinuteIndex:            minuteIndex,
				InvocationIndex:        invocationIndex,
				SuccessCount:           &successfullInvocations,
				FailedCount:            &failedInvocations,
				ApproximateFailedCount: &approximateFailedCount,
				RecordOutputChannel:    recordOutputChannel,
				AnnounceDoneWG:         &waitForInvocations,
			})
		} else {
			// To be used from within the Golang testing framework
			log.Debugf("Test mode invocation fired.\n")

			recordOutputChannel <- &mc.ExecutionRecord{
				Phase:        int(currentPhase),
				InvocationID: composeInvocationID(minuteIndex, invocationIndex),
				StartTime:    time.Now().UnixNano(),
			}

			successfullInvocations++
		}

		iat := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond

		currentTime := time.Now()
		schedulingDelay := currentTime.Sub(startOfMinute).Microseconds() - previousIATSum
		sleepFor := iat.Microseconds() - schedulingDelay
		time.Sleep(time.Duration(sleepFor) * time.Microsecond)

		previousIATSum += iat.Microseconds()

		invocationIndex++
		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex || hasMinuteExpired(startOfMinute) {
			readyToBreak := d.proceedToNextMinute(function, &minuteIndex, &invocationIndex, &startOfMinute,
				false, &currentPhase, &approximateFailedCount, &previousIATSum)

			if readyToBreak {
				break
			}
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessful, successfullInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
	atomic.AddInt64(totalIssued, numberOfIssuedInvocations)
}

func (d *Driver) proceedToNextMinute(function *common.Function, minuteIndex *int, invocationIndex *int, startOfMinute *time.Time,
	skipMinute bool, currentPhase *common.ExperimentPhase, approximateFailedCount *int64, previousIATSum *int64) bool {

	if !isRequestTargetAchieved(function.InvocationStats.Invocations[*minuteIndex], *invocationIndex, common.RequestedVsIssued) {
		// Not fatal because we want to keep the measurements to be written to the output file
		log.Warnf("Relative difference between requested and issued number of invocations is greater than %.2f%%. Terminating experiment!\n", common.RequestedVsIssuedTerminateThreshold*100)

		return true
	}

	notFailedCount := function.InvocationStats.Invocations[*minuteIndex] - int(atomic.LoadInt64(approximateFailedCount))
	if !isRequestTargetAchieved(function.InvocationStats.Invocations[*minuteIndex], notFailedCount, common.IssuedVsFailed) {
		// Not fatal because we want to keep the measurements to be written to the output file
		log.Warnf("Percentage of failed request is greater than %.2f%%. Terminating experiment!\n", common.FailedTerminateThreshold*100)

		// NOTE: approximateFailedCount is the number of requests that experienced connection timeout or
		// function timeout. If an invocation is invoked after 55th second of the minute, the connection
		// timeout will happen in the next minute, or in case of function timeout, will happen after 15
		// minutes. Hence, this metrics shows how much invocations failed in the current minute. It will
		// eventually start to grow and after the relative difference between invoked and faild goes above
		// 20% the experiment will be terminated.

		return true
	}

	*minuteIndex++
	*invocationIndex = 0
	*previousIATSum = 0
	atomic.StoreInt64(approximateFailedCount, 0)

	if d.Configuration.WithWarmup() && *minuteIndex == (d.Configuration.LoaderConfiguration.WarmupDuration+1) {
		*currentPhase = common.ExecutionPhase
		log.Infof("Warmup phase has finished. Starting the execution phase.")
	}

	if !skipMinute {
		*startOfMinute = time.Now()
	} else {
		*startOfMinute = time.Now().Add(time.Minute)
	}

	return false
}

func isRequestTargetAchieved(ideal int, real int, assertType common.RuntimeAssertType) bool {
	if ideal == 0 {
		return true
	}

	ratio := float64(ideal-real) / float64(ideal)

	var warnBound float64
	var terminationBound float64
	var warnMessage string

	switch assertType {
	case common.RequestedVsIssued:
		warnBound = common.RequestedVsIssuedWarnThreshold
		terminationBound = common.RequestedVsIssuedTerminateThreshold
		warnMessage = fmt.Sprintf("Relative difference between requested and issued number of invocations has reached %.2f.", ratio)
	case common.IssuedVsFailed:
		warnBound = common.FailedWarnThreshold
		terminationBound = common.FailedTerminateThreshold
		warnMessage = fmt.Sprintf("Percentage of failed invocations within a minute has reached %.2f.", ratio)
	default:
		log.Fatal("Invalid type of assertion at runtime.")
	}

	if ratio < 0 || ratio > 1 {
		log.Fatalf("Invalid arguments provided to runtime assertion.\n")
	} else if ratio >= terminationBound {
		return false
	}

	if ratio >= warnBound && ratio < terminationBound {
		log.Warn(warnMessage)
	}

	return true
}

func hasMinuteExpired(t1 time.Time) bool {
	return time.Since(t1) > time.Minute
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
	signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup, totalIssuedChannel chan int64) {

	// NOTE: totalNumberOfInvocations is initialized to MaxInt64 not to allow collector to complete before
	// the end signal is received on totalIssuedChannel, which deliver the total number of issued invocations.
	// This number is known once all the individual function drivers finish issuing invocations and
	// when all the invocations return
	var totalNumberOfInvocations int64 = math.MaxInt64
	var currentlyWritten int64

	invocationFile, err := os.Create(filename)
	common.Check(err)
	defer invocationFile.Close()

	_, err = invocationFile.WriteString(
		"phase," +
			"instance," +
			"invocationID," +
			"startTime," +
			"requestedDuration," +
			"responseTime," +
			"actualDuration," +
			"actualMemoryUsage," +
			"memoryAllocationTimeout," +
			"connectionTimeout," +
			"functionTimeout\n")

	if err != nil {
		log.Fatalf(err.Error())
	}

	signalReady.Done()

	for {
		select {
		case record := <-collector:
			_, err = invocationFile.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d,%d,%d,%d,%t,%t,%t\n",
				record.Phase,
				record.Instance,
				record.InvocationID,
				record.StartTime,
				record.RequestedDuration,
				record.ResponseTime,
				record.ActualDuration,
				record.ActualMemoryUsage,
				record.MemoryAllocationTimeout,
				record.ConnectionTimeout,
				record.FunctionTimeout,
			))

			if err != nil {
				log.Fatalf(err.Error())
			}

			currentlyWritten++
			if currentlyWritten == totalNumberOfInvocations {
				(*signalEverythingWritten).Done()

				break
			}
		case record := <-totalIssuedChannel:
			totalNumberOfInvocations = record
			if currentlyWritten == totalNumberOfInvocations {
				(*signalEverythingWritten).Done()

				break
			}
		}
	}

	log.Debugf("Metrics collector has exited.\n")
}

func (d *Driver) startBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (*sync.WaitGroup, chan *mc.ExecutionRecord, chan int64) {
	auxiliaryProcessBarrier := &sync.WaitGroup{}

	if d.Configuration.LoaderConfiguration.EnableMetricsScrapping {
		auxiliaryProcessBarrier.Add(3)

		// TODO: the following three go routines are untested
		// TODO: the following arguments should be configurable
		go d.CreateKnativeMetricsScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateKnativeStateUpdateScrapper(time.Second*15, auxiliaryProcessBarrier)
		go d.CreateColdStartCountScrapper(time.Second*60, auxiliaryProcessBarrier)
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan *mc.ExecutionRecord)
	totalIssuedChannel := make(chan int64)
	go d.createGlobalMetricsCollector(d.OutputFilename, globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten, totalIssuedChannel)

	traceDurationInMinutes := d.Configuration.TraceDuration
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector, totalIssuedChannel
}

func (d *Driver) internalRun() {
	var successfulInvocations int64
	var failedInvocations int64
	var invocationsIssued int64

	allIndividualDriversCompleted := sync.WaitGroup{}
	allRecordsWritten := sync.WaitGroup{}
	allRecordsWritten.Add(1)

	backgroundProcessesInitializationBarrier, globalMetricsCollector, totalIssuedChannel := d.startBackgroundProcesses(&allRecordsWritten)

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
		allIndividualDriversCompleted.Add(1)

		go d.individualFunctionDriver(
			function,
			&allIndividualDriversCompleted,
			&successfulInvocations,
			&failedInvocations,
			&invocationsIssued,
			globalMetricsCollector,
		)
	}

	allIndividualDriversCompleted.Wait()
	if atomic.LoadInt64(&successfulInvocations)+atomic.LoadInt64(&failedInvocations) != 0 {
		log.Debugf("Waiting for all the invocations record to be written.\n")
		totalIssuedChannel <- atomic.LoadInt64(&invocationsIssued)
		allRecordsWritten.Wait()
	}

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d\n", atomic.LoadInt64(&successfulInvocations))
	log.Infof("Number of failed invocations: \t%d\n", atomic.LoadInt64(&failedInvocations))
}

func (d *Driver) RunExperiment() {
	if d.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(d.Configuration.Functions)
	}

	trace.ApplyResourceLimits(d.Configuration.Functions)

	DeployFunctions(d.Configuration.Functions,
		d.Configuration.YAMLPath,
		d.Configuration.LoaderConfiguration.IsPartiallyPanic,
		d.Configuration.LoaderConfiguration.EndpointPort)

	d.internalRun()
}
