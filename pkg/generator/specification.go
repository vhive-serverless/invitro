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

package generator

import (
	"container/list"
	"math/rand"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

type SpecificationGenerator struct {
	iatRand  *rand.Rand
	specRand *rand.Rand
}

func NewSpecificationGenerator(seed int64) *SpecificationGenerator {
	return &SpecificationGenerator{
		iatRand:  rand.New(rand.NewSource(seed)),
		specRand: rand.New(rand.NewSource(seed)),
	}
}

//////////////////////////////////////////////////
// IAT GENERATION
//////////////////////////////////////////////////

// generateIATPerGranularity generates IAT for one minute based on given number of invocations and the given distribution
func (s *SpecificationGenerator) generateIATPerGranularity(numberOfInvocations int, iatDistribution common.IatDistribution, shiftIAT bool, granularity common.TraceGranularity) ([]float64, float64) {
	if numberOfInvocations == 0 {
		return []float64{}, 0.0
	}

	var iatResult []float64
	totalDuration := 0.0 // total non-scaled duration

	for i := 0; i < numberOfInvocations; i++ {
		var iat float64

		switch iatDistribution {
		case common.Exponential:
			// NOTE: Serverless in the Wild - pg. 6, paragraph 1
			iat = s.iatRand.ExpFloat64()
		case common.Uniform:
			iat = s.iatRand.Float64()
		case common.Equidistant:
			equalDistance := common.OneSecondInMicroseconds / float64(numberOfInvocations)
			if granularity == common.MinuteGranularity {
				equalDistance *= 60.0
			}

			iat = equalDistance
		default:
			log.Fatal("Unsupported IAT distribution.")
		}

		if iat == 0 {
			// No nanoseconds-level granularity, only microsecond
			log.Fatal("Generated IAT is equal to zero (unsupported). Consider increasing the clock precision.")
		}

		iatResult = append(iatResult, iat)
		totalDuration += iat
	}

	if iatDistribution == common.Uniform || iatDistribution == common.Exponential {
		// Uniform: 		we need to scale IAT from [0, 1) to [0, 60 seconds)
		// Exponential: 	we need to scale IAT from [0, +MaxFloat64) to [0, 60 seconds)
		for i := 0; i < len(iatResult); i++ {
			// how much does the IAT contributes to the total IAT sum
			iatResult[i] = iatResult[i] / totalDuration
			// convert relative contribution to absolute on 60 second interval
			iatResult[i] = iatResult[i] * common.OneSecondInMicroseconds

			if granularity == common.MinuteGranularity {
				iatResult[i] *= 60.0
			}
		}
	}

	if shiftIAT {
		// Cut the IAT array at random place to move the first invocation from the beginning of the minute
		split := s.iatRand.Float64() * common.OneSecondInMicroseconds
		if granularity == common.MinuteGranularity {
			split *= 60.0
		}
		sum, i := 0.0, 0
		for ; i < len(iatResult); i++ {
			sum += iatResult[i]
			if sum > split {
				break
			}
		}
		beginningIAT := sum - split
		endIAT := iatResult[i] - beginningIAT
		finalIAT := append([]float64{beginningIAT}, iatResult[i+1:]...)
		finalIAT = append(finalIAT, iatResult[:i]...)
		iatResult = append(finalIAT, endIAT)
	} else {
		iatResult = append([]float64{0.0}, iatResult...)
	}

	return iatResult, totalDuration
}

// GenerateIAT generates IAT according to the given distribution. Number of minutes is the length of invocationsPerMinute array
func (s *SpecificationGenerator) generateIAT(invocationsPerMinute []int, iatDistribution common.IatDistribution, shiftIAT bool, granularity common.TraceGranularity) (common.IATMatrix, common.ProbabilisticDuration) {
	var IAT [][]float64
	var nonScaledDuration []float64

	numberOfMinutes := len(invocationsPerMinute)
	for i := 0; i < numberOfMinutes; i++ {
		minuteIAT, duration := s.generateIATPerGranularity(invocationsPerMinute[i], iatDistribution, shiftIAT, granularity)

		IAT = append(IAT, minuteIAT)
		nonScaledDuration = append(nonScaledDuration, duration)
	}

	return IAT, nonScaledDuration
}

func (s *SpecificationGenerator) GenerateInvocationData(function *common.Function, iatDistribution common.IatDistribution, shiftIAT bool, granularity common.TraceGranularity) *common.FunctionSpecification {
	invocationsPerMinute := function.InvocationStats.Invocations

	// Generating IAT
	iat, rawDuration := s.generateIAT(invocationsPerMinute, iatDistribution, shiftIAT, granularity)

	// Generating runtime specifications
	var runtimeMatrix common.RuntimeSpecificationMatrix
	for i := 0; i < len(invocationsPerMinute); i++ {
		var row []common.RuntimeSpecification

		for j := 0; j < invocationsPerMinute[i]; j++ {
			row = append(row, s.generateExecutionSpecs(function))
		}

		runtimeMatrix = append(runtimeMatrix, row)
	}

	return &common.FunctionSpecification{
		IAT:                  iat,
		RawDuration:          rawDuration,
		RuntimeSpecification: runtimeMatrix,
	}
}

//////////////////////////////////////////////////
// RUNTIME AND MEMORY GENERATION
//////////////////////////////////////////////////

// Choose a random number in between. Not thread safe.
func (s *SpecificationGenerator) randIntBetween(min, max float64) int {
	intMin, intMax := int(min), int(max)

	if intMax < intMin {
		log.Fatal("Invalid runtime/memory specification.")
	}

	if intMax == intMin {
		return intMin
	} else {
		return s.specRand.Intn(intMax-intMin) + intMin
	}
}

// Should be called only when specRand is locked with its mutex
func (s *SpecificationGenerator) determineExecutionSpecSeedQuantiles() (float64, float64) {
	//* Generate uniform quantiles in [0, 1).
	runQtl := s.specRand.Float64()
	memQtl := s.specRand.Float64()

	return runQtl, memQtl
}

// Should be called only when specRand is locked with its mutex
func (s *SpecificationGenerator) generateExecuteSpec(runQtl float64, runStats *common.FunctionRuntimeStats) (runtime int) {
	switch {
	case runQtl == 0:
		runtime = int(runStats.Percentile0)
	case runQtl <= 0.01:
		runtime = s.randIntBetween(runStats.Percentile0, runStats.Percentile1)
	case runQtl <= 0.25:
		runtime = s.randIntBetween(runStats.Percentile1, runStats.Percentile25)
	case runQtl <= 0.50:
		runtime = s.randIntBetween(runStats.Percentile25, runStats.Percentile50)
	case runQtl <= 0.75:
		runtime = s.randIntBetween(runStats.Percentile50, runStats.Percentile75)
	case runQtl <= 0.99:
		runtime = s.randIntBetween(runStats.Percentile75, runStats.Percentile99)
	case runQtl < 1:
		runtime = s.randIntBetween(runStats.Percentile99, runStats.Percentile100)
	}

	return runtime
}

// Should be called only when specRand is locked with its mutex
func (s *SpecificationGenerator) generateMemorySpec(memQtl float64, memStats *common.FunctionMemoryStats) (memory int) {
	switch {
	case memQtl <= 0.01:
		memory = int(memStats.Percentile1)
	case memQtl <= 0.05:
		memory = s.randIntBetween(memStats.Percentile1, memStats.Percentile5)
	case memQtl <= 0.25:
		memory = s.randIntBetween(memStats.Percentile5, memStats.Percentile25)
	case memQtl <= 0.50:
		memory = s.randIntBetween(memStats.Percentile25, memStats.Percentile50)
	case memQtl <= 0.75:
		memory = s.randIntBetween(memStats.Percentile50, memStats.Percentile75)
	case memQtl <= 0.95:
		memory = s.randIntBetween(memStats.Percentile75, memStats.Percentile95)
	case memQtl <= 0.99:
		memory = s.randIntBetween(memStats.Percentile95, memStats.Percentile99)
	case memQtl < 1:
		memory = s.randIntBetween(memStats.Percentile99, memStats.Percentile100)
	}

	return memory
}

func (s *SpecificationGenerator) generateExecutionSpecs(function *common.Function) common.RuntimeSpecification {
	runStats, memStats := function.RuntimeStats, function.MemoryStats
	if runStats.Count <= 0 || memStats.Count <= 0 {
		log.Fatal("Invalid duration or memory specification of the function '" + function.Name + "'.")
	}

	runQtl, memQtl := s.determineExecutionSpecSeedQuantiles()
	runtime := common.MinOf(common.MaxExecTimeMilli, common.MaxOf(common.MinExecTimeMilli, s.generateExecuteSpec(runQtl, runStats)))
	memory := common.MinOf(common.MaxMemQuotaMib, common.MaxOf(common.MinMemQuotaMib, s.generateMemorySpec(memQtl, memStats)))

	return common.RuntimeSpecification{
		Runtime: runtime,
		Memory:  memory,
	}
}

func (s *SpecificationGenerator) CreateDAGWorkflow(functionList []*common.Function, function *common.Function, maxWidth int, maxDepth int) *list.List {
	DAGList := list.New()
	if maxDepth == 1 {
		DAGList.PushBack(&common.Node{Function: function, Depth: 0})
		return DAGList
	}
	widthList := s.generateNodeDistribution(maxWidth, maxDepth)
	// Implement a FIFO queue for nodes to assign functions and branches to each node.
	nodeQueue := []*list.Element{}
	for i := 0; i < len(widthList); i++ {
		widthList[i] -= 1
		DAGList.PushBack(&common.Node{Depth: -1})
	}
	var functionID int = s.getName(function)
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
			nodeList, nodeQueue = s.addBranches(nodeQueue, widthList, node, functionList, functionID)
			functionID = (functionID + len(nodeList)) % len(functionList)
		} else {
			nodeList = []*list.List{}
		}
		node.Branches = nodeList
	}
	return DAGList
}

func (s *SpecificationGenerator) getName(function *common.Function) int {
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

func (s *SpecificationGenerator) addBranches(nodeQueue []*list.Element, widthList []int, node *common.Node, functionList []*common.Function, functionID int) ([]*list.List, []*list.Element) {
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
		newBranch := s.createNewBranch(functionList, node, len(widthList), functionID)
		functionID = (functionID + 1) % len(functionList)
		nodeList[i] = newBranch
		nodeQueue = append(nodeQueue, newBranch.Front())
	}
	return nodeList, nodeQueue
}

func (s *SpecificationGenerator) createNewBranch(functionList []*common.Function, node *common.Node, maxDepth int, functionID int) *list.List {
	DAGBranch := list.New()
	// Ensuring that each node is connected to a child until the maximum depth
	for i := node.Depth + 1; i < maxDepth; i++ {
		DAGBranch.PushBack(&common.Node{Depth: -1})
	}
	child := &common.Node{Function: functionList[functionID], Depth: node.Depth + 1}
	DAGBranch.Front().Value = child
	return DAGBranch
}

func (s *SpecificationGenerator) generateNodeDistribution(maxWidth int, maxDepth int) []int {
	// Generating the number of nodes per depth (stage).
	widthList := []int{}
	widthList = append(widthList, 1)
	for i := 1; i < maxDepth-1; i++ {
		widthList = append(widthList, (rand.Intn(maxWidth-widthList[i-1]+1) + widthList[i-1]))
	}
	widthList = append(widthList, maxWidth)
	return widthList
}
