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
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver/clients"
	"github.com/vhive-serverless/loader/pkg/driver/deployment"
	"github.com/vhive-serverless/loader/pkg/driver/failure"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/generator"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"github.com/vhive-serverless/loader/pkg/trace"
)

type Driver struct {
	Configuration          *config.Configuration
	SpecificationGenerator *generator.SpecificationGenerator
	Invoker                clients.Invoker

	AsyncRecords          *common.LockFreeQueue[*mc.ExecutionRecord]
	readOpenWhiskMetadata sync.Mutex
	allFunctionsInvoked   sync.WaitGroup
}

func NewDriver(driverConfig *config.Configuration) *Driver {
	d := &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.LoaderConfiguration.Seed),

		AsyncRecords:          common.NewLockFreeQueue[*mc.ExecutionRecord](),
		readOpenWhiskMetadata: sync.Mutex{},
		allFunctionsInvoked:   sync.WaitGroup{},
	}

	d.Invoker = clients.CreateInvoker(driverConfig, &d.allFunctionsInvoked, &d.readOpenWhiskMetadata)

	return d
}

// ///////////////////////////////////////
// HELPER METHODS
// ///////////////////////////////////////
func (d *Driver) outputFilename(name string) string {
	return fmt.Sprintf("%s_%s_%d.csv", d.Configuration.LoaderConfiguration.OutputPathPrefix, name, d.Configuration.TraceDuration)
}

/////////////////////////////////////////
// DRIVER LOGIC
/////////////////////////////////////////

type InvocationMetadata struct {
	RootFunction *list.List
	Phase        common.ExperimentPhase

	InvocationID string
	IatIndex     int

	SuccessCount        *int64
	FailedCount         *int64
	FunctionsInvoked    *int64
	RecordOutputChannel chan *mc.ExecutionRecord
	AnnounceDoneWG      *sync.WaitGroup
	AnnounceDoneExe     *sync.WaitGroup
}

func composeInvocationID(timeGranularity common.TraceGranularity, minuteIndex int, invocationIndex int) string {
	var timePrefix string

	switch timeGranularity {
	case common.MinuteGranularity:
		timePrefix = "min"
	case common.SecondGranularity:
		timePrefix = "sec"
	default:
		log.Fatal("Invalid trace granularity parameter.")
	}

	return fmt.Sprintf("%s%d.inv%d", timePrefix, minuteIndex, invocationIndex)
}

func (d *Driver) invokeFunction(metadata *InvocationMetadata) {
	defer metadata.AnnounceDoneWG.Done()

	var success bool
	node := metadata.RootFunction.Front()
	var record *mc.ExecutionRecord
	var runtimeSpecifications *common.RuntimeSpecification
	var branches []*list.List
	var invocationRetries int
	for node != nil {
		function := node.Value.(*common.Node).Function
		runtimeSpecifications = &function.Specification.RuntimeSpecification[metadata.IatIndex]

		success, record = d.Invoker.Invoke(function, runtimeSpecifications)

		if !success && (d.Configuration.LoaderConfiguration.DAGMode && invocationRetries == 0) {
			log.Debugf("Invocation with for function %s with ID %s failed. Retrying Invocation", function.Name, metadata.InvocationID)
			invocationRetries += 1
			continue
		}
		record.Phase = int(metadata.Phase)
		record.Instance = fmt.Sprintf("%s%s", node.Value.(*common.Node).DAG, record.Instance)
		record.InvocationID = metadata.InvocationID

		if d.Configuration.DirigentConfiguration != nil &&
			d.Configuration.DirigentConfiguration.AsyncMode && record.AsyncResponseID != "" {
			record.TimeToSubmitMs = record.ResponseTime
			d.AsyncRecords.Enqueue(record)
		} else {
			metadata.RecordOutputChannel <- record
		}
		atomic.AddInt64(metadata.FunctionsInvoked, 1)
		if !success {
			log.Errorf("Invocation with for function %s with ID %s failed.", function.Name, metadata.InvocationID)
			atomic.AddInt64(metadata.FailedCount, 1)
			break
		}
		atomic.AddInt64(metadata.SuccessCount, 1)
		branches = node.Value.(*common.Node).Branches
		for i := 0; i < len(branches); i++ {
			newMetadataValue := *metadata
			newMetadata := &newMetadataValue
			newMetadata.RootFunction = branches[i]
			newMetadata.AnnounceDoneWG.Add(1)
			go d.invokeFunction(newMetadata)
		}

		node = node.Next()
	}
}

func (d *Driver) functionsDriver(functionLinkedList *list.List, announceFunctionDone *sync.WaitGroup, addInvocationsToGroup *sync.WaitGroup, totalSuccessful *int64, totalFailed *int64, totalIssued *int64, recordOutputChannel chan *mc.ExecutionRecord) {
	defer announceFunctionDone.Done()

	function := functionLinkedList.Front().Value.(*common.Node).Function
	invocationCount := len(function.Specification.IAT)
	addInvocationsToGroup.Add(invocationCount)

	if invocationCount == 0 {
		log.Debugf("No invocations found for function %s.\n", function.Name)
		return
	}

	// result statistics
	minuteIndexSearch := common.NewIntervalSearch(function.Specification.PerMinuteCount)
	interval := minuteIndexSearch.SearchInterval(0)
	minuteIndexEnd, minuteIndex, invocationSinceTheBeginningOfMinute := interval.End, interval.Value, 0

	IAT := function.Specification.IAT
	iatIndex, terminationIAT := 0, invocationCount

	var successfulInvocations int64
	var failedInvocations int64
	var functionsInvoked int64
	var currentPhase = common.ExecutionPhase

	waitForInvocations := sync.WaitGroup{}

	if d.Configuration.WithWarmup() {
		currentPhase = common.WarmupPhase
		log.Infof("Warmup phase has started.")
	}

	startOfExperiment := time.Now()
	var previousIATSum int64

	for {
		if iatIndex >= len(IAT) || iatIndex >= terminationIAT {
			break // end of experiment for this individual function driver
		}

		d.announceWarmupEnd(minuteIndex, &currentPhase)

		iat := time.Duration(IAT[iatIndex]) * time.Microsecond

		schedulingDelay := time.Since(startOfExperiment).Microseconds() - previousIATSum
		sleepFor := iat.Microseconds() - schedulingDelay
		time.Sleep(time.Duration(sleepFor) * time.Microsecond)

		previousIATSum += iat.Microseconds()

		if !d.Configuration.TestMode {
			waitForInvocations.Add(1)
			go d.invokeFunction(&InvocationMetadata{
				RootFunction:        functionLinkedList,
				Phase:               currentPhase,
				InvocationID:        composeInvocationID(d.Configuration.TraceGranularity, minuteIndex, invocationSinceTheBeginningOfMinute),
				IatIndex:            iatIndex,
				SuccessCount:        &successfulInvocations,
				FailedCount:         &failedInvocations,
				FunctionsInvoked:    &functionsInvoked,
				RecordOutputChannel: recordOutputChannel,
				AnnounceDoneWG:      &waitForInvocations,
				AnnounceDoneExe:     addInvocationsToGroup,
			})
		} else {
			// To be used from within the Golang testing framework
			invocationID := composeInvocationID(d.Configuration.TraceGranularity, minuteIndex, invocationSinceTheBeginningOfMinute)
			log.Debugf("Test mode invocation fired - ID = %s.\n", invocationID)

			recordOutputChannel <- &mc.ExecutionRecord{
				ExecutionRecordBase: mc.ExecutionRecordBase{
					Phase:        int(currentPhase),
					InvocationID: invocationID,
					StartTime:    time.Now().UnixNano(),
				},
			}
			functionsInvoked++
			successfulInvocations++
		}

		iatIndex++

		// counter updates
		invocationSinceTheBeginningOfMinute++
		if iatIndex > minuteIndexEnd {
			interval = minuteIndexSearch.SearchInterval(iatIndex)
			if interval != nil { // otherwise, the experiment will terminate in the next for loop iteration
				minuteIndexEnd, minuteIndex, invocationSinceTheBeginningOfMinute = interval.End, interval.Value, 0
			}
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)

	atomic.AddInt64(totalSuccessful, successfulInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
	atomic.AddInt64(totalIssued, int64(functionsInvoked))
}

func (d *Driver) announceWarmupEnd(minuteIndex int, currentPhase *common.ExperimentPhase) {
	if *currentPhase == common.WarmupPhase && minuteIndex >= d.Configuration.LoaderConfiguration.WarmupDuration {
		*currentPhase = common.ExecutionPhase
		log.Infof("Warmup phase has finished. Starting the execution phase.")
	}
}

// TODO: currently unused - add issued/requested monitoring feature
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

func (d *Driver) startBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (*sync.WaitGroup, chan *mc.ExecutionRecord, chan int64, chan int) {
	auxiliaryProcessBarrier := &sync.WaitGroup{}

	finishCh := make(chan int, 1)

	if d.Configuration.LoaderConfiguration.EnableMetricsScrapping {
		auxiliaryProcessBarrier.Add(1)

		allRecordsWritten.Add(1)
		metricsScrapper := d.CreateMetricsScrapper(time.Second*time.Duration(d.Configuration.LoaderConfiguration.MetricScrapingPeriodSeconds), auxiliaryProcessBarrier, finishCh, allRecordsWritten)
		go metricsScrapper()
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan *mc.ExecutionRecord)
	totalIssuedChannel := make(chan int64)
	go mc.CreateGlobalMetricsCollector(d.outputFilename("duration"), globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten, totalIssuedChannel)

	traceDurationInMinutes := d.Configuration.TraceDuration
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector, totalIssuedChannel, finishCh
}

func (d *Driver) internalRun() {
	var successfulInvocations int64
	var failedInvocations int64
	var invocationsIssued int64

	allFunctionsInvoked := sync.WaitGroup{}
	allIndividualDriversCompleted := sync.WaitGroup{}
	allRecordsWritten := sync.WaitGroup{}
	allRecordsWritten.Add(1)

	backgroundProcessesInitializationBarrier, globalMetricsCollector, totalIssuedChannel, scraperFinishCh := d.startBackgroundProcesses(&allRecordsWritten)
	backgroundProcessesInitializationBarrier.Wait()

	if d.Configuration.LoaderConfiguration.DAGMode {
		functions := d.Configuration.Functions
		dagLists := generator.GenerateDAGs(d.Configuration.LoaderConfiguration, functions, false)
		log.Infof("Starting DAG invocation driver\n")
		for i := range len(dagLists) {
			allIndividualDriversCompleted.Add(1)
			go d.functionsDriver(
				dagLists[i],
				&allIndividualDriversCompleted,
				&allFunctionsInvoked,
				&successfulInvocations,
				&failedInvocations,
				&invocationsIssued,
				globalMetricsCollector,
			)
		}
	} else {
		log.Infof("Starting function invocation driver\n")
		for _, function := range d.Configuration.Functions {
			allIndividualDriversCompleted.Add(1)
			functionLinkedList := list.New()
			functionLinkedList.PushBack(&common.Node{Function: function, Depth: 0})
			go d.functionsDriver(
				functionLinkedList,
				&allIndividualDriversCompleted,
				&allFunctionsInvoked,
				&successfulInvocations,
				&failedInvocations,
				&invocationsIssued,
				globalMetricsCollector,
			)
		}
	}
	allIndividualDriversCompleted.Wait()
	if atomic.LoadInt64(&successfulInvocations)+atomic.LoadInt64(&failedInvocations) != 0 {
		log.Debugf("Waiting for all the invocations record to be written.\n")

		if d.Configuration.DirigentConfiguration != nil && d.Configuration.DirigentConfiguration.AsyncMode {
			sleepFor := time.Duration(d.Configuration.DirigentConfiguration.AsyncWaitToCollectMin) * time.Minute

			log.Infof("Sleeping for %v...", sleepFor)
			time.Sleep(sleepFor)

			d.writeAsyncRecordsToLog(globalMetricsCollector)
		}
		totalIssuedChannel <- atomic.LoadInt64(&invocationsIssued)
		scraperFinishCh <- 0 // Ask the scraper to finish metrics collection

		allRecordsWritten.Wait()
	}

	statSuccess := atomic.LoadInt64(&successfulInvocations)
	statFailed := atomic.LoadInt64(&failedInvocations)

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d", statSuccess)
	log.Infof("Number of failed invocations: \t%d", statFailed)
	log.Infof("Total invocations: \t\t\t%d", statSuccess+statFailed)
	log.Infof("Failure rate: \t\t\t%.2f%%", float64(statFailed)*100.0/float64(statSuccess+statFailed))
}

func (d *Driver) GenerateSpecification() {
	log.Info("Generating IAT and runtime specifications for all the functions")

	for i, function := range d.Configuration.Functions {
		// Equalising all the InvocationStats to the first function
		if d.Configuration.LoaderConfiguration.DAGMode {
			function.InvocationStats.Invocations = d.Configuration.Functions[0].InvocationStats.Invocations
		}
		spec := d.SpecificationGenerator.GenerateInvocationData(
			function,
			d.Configuration.IATDistribution,
			d.Configuration.ShiftIAT,
			d.Configuration.TraceGranularity,
		)

		d.Configuration.Functions[i].Specification = spec
	}
}

func (d *Driver) outputIATsToFile() {
	for i, function := range d.Configuration.Functions {
		file, _ := json.MarshalIndent(function.Specification, "", " ")
		err := os.WriteFile("iat"+strconv.Itoa(i)+".json", file, 0644)
		if err != nil {
			log.Fatalf("Writing the loader config file failed: %s", err)
		}
	}
}

func (d *Driver) ReadOrWriteFileSpecification(writeIATsToFile bool, readIATsFromFile bool) {
	if writeIATsToFile && readIATsFromFile {
		log.Fatal("Invalid loader configuration. No point to read and write IATs within the same run.")
	}

	if writeIATsToFile {
		d.outputIATsToFile()

		log.Info("IATs have been generated. The program has exited.")
		os.Exit(0)
	}

	if readIATsFromFile {
		for i := range d.Configuration.Functions {
			var spec common.FunctionSpecification

			iatFile, _ := os.ReadFile("iat" + strconv.Itoa(i) + ".json")
			err := json.Unmarshal(iatFile, &spec)
			if err != nil {
				log.Fatalf("Failed to unmarshal IAT file: %s", err)
			}

			d.Configuration.Functions[i].Specification = &spec
		}
	}
}

func (d *Driver) RunExperiment() {
	if d.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(d.Configuration.Functions)
	}

	trace.ApplyResourceLimits(d.Configuration.Functions, d.Configuration.LoaderConfiguration.CPULimit)

	deployer := deployment.CreateDeployer(d.Configuration)
	deployer.Deploy(d.Configuration)

	go failure.ScheduleFailure(d.Configuration.LoaderConfiguration.Platform, d.Configuration.FailureConfiguration)

	// Generate load
	d.internalRun()

	// Clean up
	deployer.Clean()
}
