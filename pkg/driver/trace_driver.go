package driver

import (
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
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
	EnableMetricsCollection bool
	IATDistribution         common.IatDistribution
	PathToTrace             string
	TraceDuration           int // in minutes

	YAMLPath         string
	IsPartiallyPanic bool
	EndpointPort     int

	WithTracing    bool
	WarmupDuration int
	Seed           int64
	TestMode       bool

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
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.Seed),
		OutputFilename:         fmt.Sprintf("data/out/exec_duration_%d.csv", driverConfig.TraceDuration),
	}
}

func (c *DriverConfiguration) WithWarmup() bool {
	if c.WarmupDuration > 0 {
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
	totalSuccessfull *int64, totalFailed *int64, totalIssued *int64, recordOutputChannel chan *mc.ExecutionRecord) {

	totalTraceDuration := d.Configuration.TraceDuration
	minuteIndex, invocationIndex := 0, 0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	var successfullInvocations int64
	var failedInvocations int64
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
	currentInvocationStart := time.Now()
	for {
		if minuteIndex >= totalTraceDuration {
			// Check whether the end of trace has been reached
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			// Sleep for a minute if there are no invocations
			d.prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, true, &currentPhase)

			time.Sleep(time.Minute)
			continue
		}

		numberOfIssuedInvocations++
		if !d.Configuration.TestMode {
			waitForInvocations.Add(1)

			go d.invokeFunction(&InvocationMetadata{
				Function:              function,
				RuntimeSpecifications: &runtimeSpecification[minuteIndex][invocationIndex],
				Phase:                 currentPhase,
				MinuteIndex:           minuteIndex,
				InvocationIndex:       invocationIndex,
				SuccessCount:          &successfullInvocations,
				FailedCount:           &failedInvocations,
				RecordOutputChannel:   recordOutputChannel,
				AnnounceDoneWG:        &waitForInvocations,
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

		sleepFor := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond
		perInvocationDrift := sleepFor.Microseconds() - time.Now().Sub(currentInvocationStart).Microseconds()
		time.Sleep(time.Duration(perInvocationDrift) * time.Microsecond)

		currentInvocationStart = time.Now()

		invocationIndex++
		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex {
			d.prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, false, &currentPhase)
		} else if hasMinuteExpired(startOfMinute) {
			if !isRequestTargetAchieved(function.InvocationStats.Invocations[minuteIndex], invocationIndex) {
				// Not fatal because we want to keep the measurements to be written to the output file
				log.Warnf("Requested vs. issued invocations divergence is greater than 20%%. Terminating experiment!\n")

				break
			}

			d.prepareForNextMinute(&minuteIndex, &invocationIndex, &startOfMinute, false, &currentPhase)
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessfull, successfullInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
	atomic.AddInt64(totalIssued, numberOfIssuedInvocations)
}

func (d *Driver) prepareForNextMinute(minuteIndex *int, invocationIndex *int, startOfMinute *time.Time, skipMinute bool, currentPhase *common.ExperimentPhase) {
	*minuteIndex++
	*invocationIndex = 0

	if d.Configuration.WithWarmup() && *minuteIndex == (d.Configuration.WarmupDuration+1) {
		*currentPhase = common.ExecutionPhase
		log.Infof("Warmup phase has finished. Starting the execution phase.")
	}

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

	// TODO: unsure if this is the best approach - program can issued new go ruites but they may not be scheduler at all
	if ratio >= 0.1 && ratio < 0.2 {
		log.Warnf("Requested vs. issued invocations divergence is %.2f.\n", ratio)
	}

	return true
}

func hasMinuteExpired(t1 time.Time) bool {
	if time.Now().Sub(t1) > time.Minute {
		return true
	}

	return false
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

	invocationFile.WriteString(
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

	signalReady.Done()

	for {
		select {
		case record := <-collector:
			invocationFile.WriteString(fmt.Sprintf("%d,%s,%s,%d,%d,%d,%d,%d,%t,%t,%t\n",
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
	totalIssuedChannel := make(chan int64)
	go d.createGlobalMetricsCollector(d.OutputFilename, globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten, totalIssuedChannel)

	traceDurationInMinutes := d.Configuration.TraceDuration
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector, totalIssuedChannel
}

func (d *Driver) internalRun() {
	var successfullInvocations int64
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
			&successfullInvocations,
			&failedInvocations,
			&invocationsIssued,
			globalMetricsCollector,
		)
	}

	allIndividualDriversCompleted.Wait()
	if successfullInvocations+failedInvocations != 0 {
		log.Debugf("Waiting for all the invocations record to be written.\n")
		totalIssuedChannel <- invocationsIssued
		allRecordsWritten.Wait()
	}

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d\n", successfullInvocations)
	log.Infof("Number of failed invocations: \t%d\n", failedInvocations)
}

func (d *Driver) RunExperiment() {
	if d.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(d.Configuration.Functions)
	}

	trace.ApplyResourceLimits(d.Configuration.Functions)

	DeployFunctions(d.Configuration.Functions,
		d.Configuration.YAMLPath,
		d.Configuration.IsPartiallyPanic,
		d.Configuration.EndpointPort)

	d.internalRun()
}
