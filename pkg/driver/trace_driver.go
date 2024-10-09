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
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/generator"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"github.com/vhive-serverless/loader/pkg/trace"
)

type DriverConfiguration struct {
	LoaderConfiguration *config.LoaderConfiguration
	IATDistribution     common.IatDistribution
	ShiftIAT            bool // shift the invocations inside minute
	TraceGranularity    common.TraceGranularity
	TraceDuration       int // in minutes

	YAMLPath string
	TestMode bool

	Functions []*common.Function
}

type Driver struct {
	Configuration          *DriverConfiguration
	SpecificationGenerator *generator.SpecificationGenerator
}

func NewDriver(driverConfig *DriverConfiguration) *Driver {
	return &Driver{
		Configuration:          driverConfig,
		SpecificationGenerator: generator.NewSpecificationGenerator(driverConfig.LoaderConfiguration.Seed),
	}
}

func (c *DriverConfiguration) WithWarmup() bool {
	if c.LoaderConfiguration.WarmupDuration > 0 {
		return true
	} else {
		return false
	}
}

// ///////////////////////////////////////
// HELPER METHODS
// ///////////////////////////////////////
func (d *Driver) outputFilename(name string) string {
	return fmt.Sprintf("%s_%s_%d.csv", d.Configuration.LoaderConfiguration.OutputPathPrefix, name, d.Configuration.TraceDuration)
}

func (d *Driver) runCSVWriter(records chan interface{}, filename string, writerDone *sync.WaitGroup) {
	log.Debugf("Starting writer for %s", filename)

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()

	writer := gocsv.NewSafeCSVWriter(csv.NewWriter(file))
	if err := gocsv.MarshalChan(records, writer); err != nil {
		log.Fatal(err)
	}

	writerDone.Done()
}

func createDAGWorkflow(functionList []*common.Function, function *common.Function, maxWidth int, maxDepth int) *list.List {
	DAGList := list.New()
	if maxDepth == 1 {
		DAGList.PushBack(&common.Node{Function: function, Depth: 0})
		return DAGList
	}
	widthList := generateNodeDistribution(maxWidth, maxDepth)
	// Implement a FIFO queue for nodes to assign functions and branches to each node.
	nodeQueue := []*list.Element{}
	for i := 0; i < len(widthList); i++ {
		widthList[i] -= 1
		DAGList.PushBack(&common.Node{Depth: -1})
	}
	var functionID int = getName(function)
	DAGList.Front().Value = &common.Node{Function: function, Depth: 0}
	functionID = (functionID + 1) % len(functionList)
	nodeQueue = append(nodeQueue, DAGList.Front())
	for len(nodeQueue) > 0 {
		listElement := nodeQueue[0]
		nodeQueue = nodeQueue[1:]
		node := listElement.Value.(*common.Node)
		// Checks if the node has reached the maximum depth of the DAG (maxDepth -1)
		if node.Depth == maxDepth-1 {
			continue
		}
		child := &common.Node{Function: functionList[functionID], Depth: node.Depth + 1}
		functionID = (functionID + 1) % len(functionList)
		listElement.Next().Value = child
		nodeQueue = append(nodeQueue, listElement.Next())
		// Creating parallel branches from the node, if width of next stage > width of current stage
		var nodeList []*list.List
		if widthList[node.Depth+1] > 0 {
			nodeList, nodeQueue = addBranches(nodeQueue, widthList, node, functionList, functionID)
			functionID = (functionID + len(nodeList)) % len(functionList)
		} else {
			nodeList = []*list.List{}
		}
		node.Branches = nodeList
	}
	return DAGList
}

func addBranches(nodeQueue []*list.Element, widthList []int, node *common.Node, functionList []*common.Function, functionID int) ([]*list.List, []*list.Element) {
	var additionalBranches int
	if len(nodeQueue) < 1 || (nodeQueue[0].Value.(*common.Node).Depth > node.Depth) {
		additionalBranches = widthList[node.Depth+1]
	} else {
		additionalBranches = rand.Intn(widthList[node.Depth+1] + 1)
	}
	for i := node.Depth + 1; i < len(widthList); i++ {
		widthList[i] -= additionalBranches
	}
	nodeList := make([]*list.List, additionalBranches)
	for i := 0; i < additionalBranches; i++ {
		newBranch := createNewBranch(functionList, node, len(widthList), functionID)
		functionID = (functionID + 1) % len(functionList)
		nodeList[i] = newBranch
		nodeQueue = append(nodeQueue, newBranch.Front())
	}
	return nodeList, nodeQueue
}

func createNewBranch(functionList []*common.Function, node *common.Node, maxDepth int, functionID int) *list.List {
	DAGBranch := list.New()
	// Ensuring that each node is connected to a child until the maximum depth
	for i := node.Depth + 1; i < maxDepth; i++ {
		DAGBranch.PushBack(&common.Node{Depth: -1})
	}
	child := &common.Node{Function: functionList[functionID], Depth: node.Depth + 1}
	DAGBranch.Front().Value = child
	return DAGBranch
}

func generateNodeDistribution(maxWidth int, maxDepth int) []int {
	// Generating the number of nodes per depth (stage).
	widthList := []int{}
	widthList = append(widthList, 1)
	for i := 1; i < maxDepth-1; i++ {
		widthList = append(widthList, (rand.Intn(maxWidth-widthList[i-1]+1) + widthList[i-1]))
	}
	widthList = append(widthList, maxWidth)
	return widthList
}

// Visual Representation for the DAG
func printDAG(DAGWorkflow *list.List) {
	DAGNode := DAGWorkflow.Front()
	nodeQueue := make([]*list.Element, 0)
	nodeQueue = append(nodeQueue, DAGNode)
	var printMessage string
	var buffer string = ""
	var dummyNode *list.Element
	var startingNode bool = true
	for len(nodeQueue) > 0 {
		DAGNode = nodeQueue[0]
		nodeQueue = nodeQueue[1:]
		functionId := getName(DAGNode.Value.(*common.Node).Function)
		if startingNode {
			printMessage = "|" + strconv.Itoa(functionId)
			for i := 0; i < DAGNode.Value.(*common.Node).Depth; i++ {
				buffer += "     "
			}
			printMessage = buffer + printMessage
			startingNode = false
		} else {
			printMessage = printMessage + " -> " + strconv.Itoa(functionId)
		}
		for i := 0; i < len(DAGNode.Value.(*common.Node).Branches); i++ {
			nodeQueue = append(nodeQueue, dummyNode)
			copy(nodeQueue[1:], nodeQueue)
			nodeQueue[0] = DAGNode.Value.(*common.Node).Branches[i].Front()
		}
		if DAGNode.Next() == nil {
			println(printMessage)
			buffer = ""
			if len(nodeQueue) > 0 {
				startingNode = true
			} else {
				break
			}
		} else {
			nodeQueue = append(nodeQueue, dummyNode)
			copy(nodeQueue[1:], nodeQueue)
			nodeQueue[0] = DAGNode.Next()
		}
	}
}

func getMaxInvocation(functionList []*common.Function) []int {
	maxInvocation := make([]int, len(functionList[0].InvocationStats.Invocations))
	for _, i := range functionList {
		for index, invocation := range i.InvocationStats.Invocations {
			maxInvocation[index] = max(maxInvocation[index], invocation)
		}
	}
	return maxInvocation
}

func getName(function *common.Function) int {
	parts := strings.Split(function.Name, "-")
	if parts[0] == "test" {
		return 0
	}
	functionId, err := strconv.Atoi(parts[2])
	if err != nil {
		log.Fatal(err)
	}
	return functionId
}

// Read the Cumulative Distribution Frequency (CDF) of the widths and depths of a DAG
func generateCDF(file string) [][]float64 {
	f, err := os.Open(file)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	csvReader := csv.NewReader(f)
	records, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	records = records[1:]
	cdf := make([][]float64, len(records[0]))
	for i := 0; i < len(records[0]); i++ {
		cdf[i] = make([]float64, len(records))
	}
	for i := 0; i < len(records[0]); i += 2 {
		for j := 0; j < len(records); j++ {
			cdfProb, _ := strconv.ParseFloat(strings.TrimSuffix(records[j][i+1], "%"), 64)
			cdfValue, _ := strconv.ParseFloat(records[j][i], 64)
			cdf[i+1][j] = cdfProb
			cdf[i][j] = cdfValue
			if cdfProb == 100.00 {
				cdf[i] = cdf[i][:j+1]
				cdf[i+1] = cdf[i+1][:j+1]
				break
			}
		}
	}
	return cdf
}

// Generate pseudo-random probabilities and compare it with the given CDF to obtain the depth and width of the DAG
func getDAGStats(cdf [][]float64, maxSize int, numberOfTries int) (int, int) {
	var width, depth int
	depthProb := rand.Float64() * 100
	widthProb := rand.Float64() * 100
	for i, value := range cdf[1] {
		if value >= widthProb {
			width = int(cdf[0][i])
			break
		}
	}
	for i, value := range cdf[3] {
		if value >= depthProb {
			depth = int(cdf[2][i])
			break
		}
	}
	// Re-run DAG Generation if size exceeds number of functions
	if maxSize < width*(depth-1)+1 {
		if numberOfTries == 10 {
			return 1, maxSize
		}
		width, depth = getDAGStats(cdf, maxSize, numberOfTries+1)
	}
	return width, depth
}

/////////////////////////////////////////
// METRICS SCRAPPERS
/////////////////////////////////////////

func (d *Driver) CreateMetricsScrapper(interval time.Duration,
	signalReady *sync.WaitGroup, finishCh chan int, allRecordsWritten *sync.WaitGroup) func() {
	timer := time.NewTicker(interval)

	return func() {
		signalReady.Done()
		knStatRecords := make(chan interface{}, 100)
		scaleRecords := make(chan interface{}, 100)
		writerDone := sync.WaitGroup{}

		clusterUsageFile, err := os.Create(d.outputFilename("cluster_usage"))
		common.Check(err)
		defer clusterUsageFile.Close()

		writerDone.Add(1)
		go d.runCSVWriter(knStatRecords, d.outputFilename("kn_stats"), &writerDone)

		writerDone.Add(1)
		go d.runCSVWriter(scaleRecords, d.outputFilename("deployment_scale"), &writerDone)

		for {
			select {
			case <-timer.C:
				recCluster := mc.ScrapeClusterUsage()
				recCluster.Timestamp = time.Now().UnixMicro()

				byteArr, err := json.Marshal(recCluster)
				common.Check(err)

				_, err = clusterUsageFile.Write(byteArr)
				common.Check(err)

				_, err = clusterUsageFile.WriteString("\n")
				common.Check(err)

				recScale := mc.ScrapeDeploymentScales()
				timestamp := time.Now().UnixMicro()
				for _, rec := range recScale {
					rec.Timestamp = timestamp
					scaleRecords <- rec
				}

				recKnative := mc.ScrapeKnStats()
				recKnative.Timestamp = time.Now().UnixMicro()
				knStatRecords <- recKnative
			case <-finishCh:
				close(knStatRecords)
				close(scaleRecords)

				writerDone.Wait()
				allRecordsWritten.Done()

				return
			}
		}
	}
}

/////////////////////////////////////////
// DRIVER LOGIC
/////////////////////////////////////////

type InvocationMetadata struct {
	RootFunction *list.List
	Phase        common.ExperimentPhase

	MinuteIndex     int
	InvocationIndex int

	SuccessCount        *int64
	FailedCount         *int64
	FailedCountByMinute []int64
	FunctionsInvoked    *int64

	RecordOutputChannel   chan interface{}
	AnnounceDoneWG        *sync.WaitGroup
	AnnounceDoneExe       *sync.WaitGroup
	ReadOpenWhiskMetadata *sync.Mutex
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
	var numberOfFunctionsInvoked int64
	for node != nil {
		function := node.Value.(*common.Node).Function
		runtimeSpecifications = &function.Specification.RuntimeSpecification[metadata.MinuteIndex][metadata.InvocationIndex]
		switch d.Configuration.LoaderConfiguration.Platform {
		case "Knative":
			success, record = InvokeGRPC(
				function,
				runtimeSpecifications,
				d.Configuration.LoaderConfiguration,
			)
		case "OpenWhisk":
			success, record = InvokeOpenWhisk(
				function,
				runtimeSpecifications,
				metadata.AnnounceDoneExe,
				metadata.ReadOpenWhiskMetadata,
			)
		case "AWSLambda":
			success, record = InvokeAWSLambda(
				function,
				runtimeSpecifications,
				metadata.AnnounceDoneExe,
			)
		case "Dirigent":
			success, record = InvokeDirigent(
				function,
				runtimeSpecifications,
				d.Configuration.LoaderConfiguration,
			)
		default:
			log.Fatal("Unsupported platform.")
		}
		if !success && (d.Configuration.LoaderConfiguration.DAGMode && invocationRetries == 0) {
			log.Debugf("Invocation failed at minute: %d for %s. Retrying Invocation", metadata.MinuteIndex, function.Name)
			invocationRetries += 1
			continue
		}
		record.Phase = int(metadata.Phase)
		record.InvocationID = composeInvocationID(d.Configuration.TraceGranularity, metadata.MinuteIndex, metadata.InvocationIndex)
		metadata.RecordOutputChannel <- record
		numberOfFunctionsInvoked += 1
		if !success {
			log.Debugf("Invocation failed at minute: %d for %s", metadata.MinuteIndex, function.Name)
			break
		}
		branches = node.Value.(*common.Node).Branches
		for i := 0; i < len(branches); i++ {
			newMetadataValue := *metadata
			newMetadata := &newMetadataValue
			newMetadata.RootFunction = branches[i]
			newMetadata.AnnounceDoneWG.Add(1)
			atomic.AddInt64(metadata.SuccessCount, -1)
			go d.invokeFunction(newMetadata)
		}
		node = node.Next()
	}
	atomic.AddInt64(metadata.FunctionsInvoked, numberOfFunctionsInvoked)
	if success {
		atomic.AddInt64(metadata.SuccessCount, 1)
	} else {
		atomic.AddInt64(metadata.FailedCount, 1)
		atomic.AddInt64(&metadata.FailedCountByMinute[metadata.MinuteIndex], 1)
	}
}

func (d *Driver) functionsDriver(functionLinkedList *list.List, announceFunctionDone *sync.WaitGroup,
	addInvocationsToGroup *sync.WaitGroup, readOpenWhiskMetadata *sync.Mutex, totalSuccessful *int64,
	totalFailed *int64, totalIssued *int64, entriesWritten *int64, recordOutputChannel chan interface{}) {

	function := functionLinkedList.Front().Value.(*common.Node).Function
	numberOfInvocations := 0
	for i := 0; i < len(function.InvocationStats.Invocations); i++ {
		numberOfInvocations += function.InvocationStats.Invocations[i]
	}
	addInvocationsToGroup.Add(numberOfInvocations)

	totalTraceDuration := d.Configuration.TraceDuration
	minuteIndex, invocationIndex := 0, 0

	IAT := function.Specification.IAT

	var successfulInvocations int64
	var failedInvocations int64
	var failedInvocationByMinute = make([]int64, totalTraceDuration)
	var numberOfIssuedInvocations int64
	var functionsInvoked int64
	var currentPhase = common.ExecutionPhase

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
			if d.proceedToNextMinute(function, &minuteIndex, &invocationIndex,
				&startOfMinute, true, &currentPhase, failedInvocationByMinute, &previousIATSum) {
				break
			}

			switch d.Configuration.TraceGranularity {
			case common.MinuteGranularity:
				time.Sleep(time.Minute)
			case common.SecondGranularity:
				time.Sleep(time.Second)
			default:
				log.Fatal("Unsupported trace granularity.")
			}

			continue
		}

		iat := time.Duration(IAT[minuteIndex][invocationIndex]) * time.Microsecond

		currentTime := time.Now()
		schedulingDelay := currentTime.Sub(startOfMinute).Microseconds() - previousIATSum
		sleepFor := iat.Microseconds() - schedulingDelay
		time.Sleep(time.Duration(sleepFor) * time.Microsecond)

		previousIATSum += iat.Microseconds()

		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex || hasMinuteExpired(startOfMinute) {
			readyToBreak := d.proceedToNextMinute(function, &minuteIndex, &invocationIndex, &startOfMinute,
				false, &currentPhase, failedInvocationByMinute, &previousIATSum)

			if readyToBreak {
				break
			}
		} else {
			if !d.Configuration.TestMode {
				waitForInvocations.Add(1)

				go d.invokeFunction(&InvocationMetadata{
					RootFunction:          functionLinkedList,
					Phase:                 currentPhase,
					MinuteIndex:           minuteIndex,
					InvocationIndex:       invocationIndex,
					SuccessCount:          &successfulInvocations,
					FailedCount:           &failedInvocations,
					FailedCountByMinute:   failedInvocationByMinute,
					FunctionsInvoked:      &functionsInvoked,
					RecordOutputChannel:   recordOutputChannel,
					AnnounceDoneWG:        &waitForInvocations,
					AnnounceDoneExe:       addInvocationsToGroup,
					ReadOpenWhiskMetadata: readOpenWhiskMetadata,
				})
			} else {
				// To be used from within the Golang testing framework
				log.Debugf("Test mode invocation fired.\n")

				recordOutputChannel <- &mc.ExecutionRecordBase{
					Phase:        int(currentPhase),
					InvocationID: composeInvocationID(d.Configuration.TraceGranularity, minuteIndex, invocationIndex),
					StartTime:    time.Now().UnixNano(),
				}
				functionsInvoked++
				successfulInvocations++
			}
			numberOfIssuedInvocations++
			invocationIndex++
		}
	}

	waitForInvocations.Wait()

	log.Debugf("All the invocations for function %s have been completed.\n", function.Name)
	announceFunctionDone.Done()

	atomic.AddInt64(totalSuccessful, successfulInvocations)
	atomic.AddInt64(totalFailed, failedInvocations)
	atomic.AddInt64(totalIssued, numberOfIssuedInvocations)
	atomic.AddInt64(entriesWritten, functionsInvoked)
}

func (d *Driver) proceedToNextMinute(function *common.Function, minuteIndex *int, invocationIndex *int, startOfMinute *time.Time,
	skipMinute bool, currentPhase *common.ExperimentPhase, failedInvocationByMinute []int64, previousIATSum *int64) bool {

	if d.Configuration.TraceGranularity == common.MinuteGranularity {
		if !isRequestTargetAchieved(function.InvocationStats.Invocations[*minuteIndex], *invocationIndex, common.RequestedVsIssued) {
			// Not fatal because we want to keep the measurements to be written to the output file
			log.Warnf("Relative difference between requested and issued number of invocations is greater than %.2f%%. Terminating function driver for %s!\n", common.RequestedVsIssuedTerminateThreshold*100, function.Name)

			return true
		}

		for i := 0; i <= *minuteIndex; i++ {
			notFailedCount := function.InvocationStats.Invocations[i] - int(atomic.LoadInt64(&failedInvocationByMinute[i]))
			if !isRequestTargetAchieved(function.InvocationStats.Invocations[i], notFailedCount, common.IssuedVsFailed) {
				// Not fatal because we want to keep the measurements to be written to the output file
				log.Warnf("Percentage of failed request is greater than %.2f%%. Terminating function driver for %s!\n", common.FailedTerminateThreshold*100, function.Name)

				return true
			}
		}
	}

	*minuteIndex++
	*invocationIndex = 0
	*previousIATSum = 0

	if d.Configuration.WithWarmup() && *minuteIndex == (d.Configuration.LoaderConfiguration.WarmupDuration+1) {
		*currentPhase = common.ExecutionPhase
		log.Infof("Warmup phase has finished. Starting the execution phase.")
	}

	if !skipMinute {
		*startOfMinute = time.Now()
	} else {
		switch d.Configuration.TraceGranularity {
		case common.MinuteGranularity:
			*startOfMinute = time.Now().Add(time.Minute)
		case common.SecondGranularity:
			*startOfMinute = time.Now().Add(time.Second)
		default:
			log.Fatal("Unsupported trace granularity.")
		}
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

func (d *Driver) createGlobalMetricsCollector(filename string, collector chan interface{},
	signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup, totalIssuedChannel chan int64) {

	// NOTE: totalNumberOfInvocations is initialized to MaxInt64 not to allow collector to complete before
	// the end signal is received on totalIssuedChannel, which deliver the total number of issued invocations.
	// This number is known once all the individual function drivers finish issuing invocations and
	// when all the invocations return
	var totalNumberOfInvocations int64 = math.MaxInt64
	var currentlyWritten int64

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()
	signalReady.Done()

	records := make(chan interface{}, 100)
	writerDone := sync.WaitGroup{}
	writerDone.Add(1)
	go d.runCSVWriter(records, filename, &writerDone)

	for {
		select {
		case record := <-collector:
			records <- record

			currentlyWritten++
		case record := <-totalIssuedChannel:
			totalNumberOfInvocations = record
		}

		if currentlyWritten == totalNumberOfInvocations {
			close(records)
			writerDone.Wait()
			(*signalEverythingWritten).Done()

			return
		}
	}
}

func (d *Driver) startBackgroundProcesses(allRecordsWritten *sync.WaitGroup) (*sync.WaitGroup, chan interface{}, chan int64, chan int) {
	auxiliaryProcessBarrier := &sync.WaitGroup{}

	finishCh := make(chan int, 1)

	if d.Configuration.LoaderConfiguration.EnableMetricsScrapping {
		auxiliaryProcessBarrier.Add(1)

		allRecordsWritten.Add(1)
		metricsScrapper := d.CreateMetricsScrapper(time.Second*time.Duration(d.Configuration.LoaderConfiguration.MetricScrapingPeriodSeconds), auxiliaryProcessBarrier, finishCh, allRecordsWritten)
		go metricsScrapper()
	}

	auxiliaryProcessBarrier.Add(2)

	globalMetricsCollector := make(chan interface{})
	totalIssuedChannel := make(chan int64)
	go d.createGlobalMetricsCollector(d.outputFilename("duration"), globalMetricsCollector, auxiliaryProcessBarrier, allRecordsWritten, totalIssuedChannel)

	traceDurationInMinutes := d.Configuration.TraceDuration
	go d.globalTimekeeper(traceDurationInMinutes, auxiliaryProcessBarrier)

	return auxiliaryProcessBarrier, globalMetricsCollector, totalIssuedChannel, finishCh
}

func (d *Driver) internalRun(iatOnly bool, generated bool) {
	var successfulInvocations int64
	var failedInvocations int64
	var invocationsIssued int64
	var entriesWritten int64
	var width int
	var depth int
	readOpenWhiskMetadata := sync.Mutex{}
	allFunctionsInvoked := sync.WaitGroup{}
	allIndividualDriversCompleted := sync.WaitGroup{}
	allRecordsWritten := sync.WaitGroup{}
	allRecordsWritten.Add(1)

	backgroundProcessesInitializationBarrier, globalMetricsCollector, totalIssuedChannel, scraperFinishCh := d.startBackgroundProcesses(&allRecordsWritten)

	if !iatOnly {
		log.Info("Generating IAT and runtime specifications for all the functions")
		maxInvocation := getMaxInvocation(d.Configuration.Functions)
		for i, function := range d.Configuration.Functions {
			spec := d.SpecificationGenerator.GenerateInvocationData(
				function,
				d.Configuration.IATDistribution,
				d.Configuration.ShiftIAT,
				d.Configuration.TraceGranularity,
			)
			d.Configuration.Functions[i].Specification = spec
			// Overwriting the runtime specification to account for maximum possible invocations
			if d.Configuration.LoaderConfiguration.DAGMode {
				originalInvocation := function.InvocationStats.Invocations
				function.InvocationStats.Invocations = maxInvocation
				spec := d.SpecificationGenerator.GenerateInvocationData(
					function,
					d.Configuration.IATDistribution,
					d.Configuration.ShiftIAT,
					d.Configuration.TraceGranularity,
				)
				function.InvocationStats.Invocations = originalInvocation
				function.Specification.RuntimeSpecification = spec.RuntimeSpecification
			}
		}
	}

	backgroundProcessesInitializationBarrier.Wait()

	if generated {
		for i := range d.Configuration.Functions {
			var spec common.FunctionSpecification

			iatFile, _ := os.ReadFile("iat" + strconv.Itoa(i) + ".json")
			err := json.Unmarshal(iatFile, &spec)
			if err != nil {
				log.Fatalf("Failed tu unmarshal iat file: %s", err)
			}

			d.Configuration.Functions[i].Specification = &spec
		}
	}

	if d.Configuration.LoaderConfiguration.DAGMode {
		DAGDistribution := generateCDF("data/traces/example/dag_structure.csv")
		totalLinkedList := []*list.List{}
		for _, function := range d.Configuration.Functions {
			if d.Configuration.LoaderConfiguration.EnableDAGDataset {
				width, depth = getDAGStats(DAGDistribution, len(d.Configuration.Functions), 0)
			} else {
				width = d.Configuration.LoaderConfiguration.Width
				depth = d.Configuration.LoaderConfiguration.Depth
				// Sanity checking if max size of DAG exceeds number of functions available
				if len(d.Configuration.Functions) < (depth-1)*width+1 {
					log.Fatalf("DAG size exceeded: Functions required: %d, Available Functions: %d", (depth-1)*width+1, len(d.Configuration.Functions))
				}
			}
			functionLinkedList := createDAGWorkflow(d.Configuration.Functions, function, width, depth)
			printDAG(functionLinkedList)
			totalLinkedList = append(totalLinkedList, functionLinkedList)
		}
		log.Infof("Starting DAG invocation driver\n")
		for _, functionLinkedList := range totalLinkedList {
			allIndividualDriversCompleted.Add(1)
			go d.functionsDriver(
				functionLinkedList,
				&allIndividualDriversCompleted,
				&allFunctionsInvoked,
				&readOpenWhiskMetadata,
				&successfulInvocations,
				&failedInvocations,
				&invocationsIssued,
				&entriesWritten,
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
				&readOpenWhiskMetadata,
				&successfulInvocations,
				&failedInvocations,
				&invocationsIssued,
				&entriesWritten,
				globalMetricsCollector,
			)
		}
	}
	allIndividualDriversCompleted.Wait()
	if atomic.LoadInt64(&successfulInvocations)+atomic.LoadInt64(&failedInvocations) != 0 {
		log.Debugf("Waiting for all the invocations record to be written.\n")

		totalIssuedChannel <- atomic.LoadInt64(&entriesWritten)
		scraperFinishCh <- 0 // Ask the scraper to finish metrics collection

		allRecordsWritten.Wait()
	}

	log.Infof("Trace has finished executing function invocation driver\n")
	log.Infof("Number of successful invocations: \t%d\n", atomic.LoadInt64(&successfulInvocations))
	log.Infof("Number of failed invocations: \t%d\n", atomic.LoadInt64(&failedInvocations))
}

func (d *Driver) RunExperiment(iatOnly bool, generated bool) {
	if iatOnly {
		log.Info("Generating IAT and runtime specifications for all the functions")
		for i, function := range d.Configuration.Functions {
			spec := d.SpecificationGenerator.GenerateInvocationData(
				function,
				d.Configuration.IATDistribution,
				d.Configuration.ShiftIAT,
				d.Configuration.TraceGranularity,
			)
			d.Configuration.Functions[i].Specification = spec

			file, _ := json.MarshalIndent(spec, "", " ")
			err := os.WriteFile("iat"+strconv.Itoa(i)+".json", file, 0644)
			if err != nil {
				log.Fatalf("Writing the loader config file failed: %s", err)
			}
		}

		return
	}

	if d.Configuration.WithWarmup() {
		trace.DoStaticTraceProfiling(d.Configuration.Functions)
	}

	trace.ApplyResourceLimits(d.Configuration.Functions, d.Configuration.LoaderConfiguration.CPULimit)

	switch d.Configuration.LoaderConfiguration.Platform {
	case "Knative":
		DeployFunctions(d.Configuration.Functions,
			d.Configuration.YAMLPath,
			d.Configuration.LoaderConfiguration.IsPartiallyPanic,
			d.Configuration.LoaderConfiguration.EndpointPort,
			d.Configuration.LoaderConfiguration.AutoscalingMetric)
	case "OpenWhisk":
		DeployFunctionsOpenWhisk(d.Configuration.Functions)
	case "AWSLambda":
		DeployFunctionsAWSLambda(d.Configuration.Functions)
	case "Dirigent":
		DeployDirigent(d.Configuration.Functions)
	default:
		log.Fatal("Unsupported platform.")
	}

	// Generate load
	d.internalRun(iatOnly, generated)

	// Clean up
	if d.Configuration.LoaderConfiguration.Platform == "Knative" {
		CleanKnative()
	} else if d.Configuration.LoaderConfiguration.Platform == "OpenWhisk" {
		CleanOpenWhisk(d.Configuration.Functions)
	} else if d.Configuration.LoaderConfiguration.Platform == "AWSLambda" {
		CleanAWSLambda(d.Configuration.Functions)
	}
}
