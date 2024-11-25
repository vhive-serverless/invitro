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
	"math/rand"

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
func (s *SpecificationGenerator) generateIATPerGranularity(minuteIndex int, numberOfInvocations int, iatDistribution common.IatDistribution, shiftIAT bool, granularity common.TraceGranularity) ([]float64, float64) {
	if numberOfInvocations == 0 {
		return []float64{}, 0.0
	}

	var iatResult []float64

	endIndex := numberOfInvocations
	totalDuration := 0.0 // total non-scaled duration

	if minuteIndex == 0 {
		iatResult = []float64{0.0}
		// -1 because the first invocation happens as soon as the experiment starts
		endIndex = numberOfInvocations - 1
	}

	for i := 0; i < endIndex; i++ {
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
	}

	return iatResult, totalDuration
}

func getBlankTimeUnit(granularity common.TraceGranularity) float64 {
	if granularity == common.MinuteGranularity {
		return 60_000_000
	} else {
		return 1_000_000
	}
}

// GenerateIAT generates IAT according to the given distribution. Number of minutes is the length of invocationsPerMinute array
func (s *SpecificationGenerator) generateIAT(invocationsPerMinute []int, iatDistribution common.IatDistribution,
	shiftIAT bool, granularity common.TraceGranularity) (common.IATArray, []int, common.ProbabilisticDuration) {

	var IAT []float64
	var perMinuteCount []int
	var nonScaledDuration []float64

	accumulatedIdle := 0.0

	numberOfMinutes := len(invocationsPerMinute)
	for i := 0; i < numberOfMinutes; i++ {
		minuteIAT, duration := s.generateIATPerGranularity(i, invocationsPerMinute[i], iatDistribution, shiftIAT, granularity)
		if len(minuteIAT) == 0 {
			accumulatedIdle += getBlankTimeUnit(granularity)
			continue
		} else if accumulatedIdle != 0 {
			IAT = append(IAT, accumulatedIdle)
			IAT = append(IAT, minuteIAT[1:]...)
			accumulatedIdle = 0.0
		} else {
			IAT = append(IAT, minuteIAT...)
		}

		perMinuteCount = append(perMinuteCount, len(minuteIAT)-1)
		nonScaledDuration = append(nonScaledDuration, duration)
	}

	return IAT, perMinuteCount, nonScaledDuration
}

func (s *SpecificationGenerator) GenerateInvocationData(function *common.Function, iatDistribution common.IatDistribution, shiftIAT bool, granularity common.TraceGranularity) *common.FunctionSpecification {
	invocationsPerMinute := function.InvocationStats.Invocations

	// Generating IAT
	iat, perMinuteCount, rawDuration := s.generateIAT(invocationsPerMinute, iatDistribution, shiftIAT, granularity)

	// Generating runtime specifications
	var runtimeArray common.RuntimeSpecificationArray
	for i := 0; i < len(perMinuteCount); i++ {
		for j := 0; j < perMinuteCount[i]; j++ {
			runtimeArray = append(runtimeArray, s.generateExecutionSpecs(function))
		}
	}

	return &common.FunctionSpecification{
		IAT:                  iat,
		PerMinuteCount:       perMinuteCount,
		RawDuration:          rawDuration,
		RuntimeSpecification: runtimeArray,
	}
}

//////////////////////////////////////////////////
// RUNTIME AND MEMORY GENERATION
//////////////////////////////////////////////////

// Choose a random number in between. Not thread safe.
func randIntBetween(gen *rand.Rand, min, max float64) int {
	intMin, intMax := int(min), int(max)

	if intMax < intMin {
		log.Fatal("Invalid runtime/memory specification.")
	}

	if intMax == intMin {
		return intMin
	} else {
		return gen.Intn(intMax-intMin) + intMin
	}
}

// Should be called only when specRand is locked with its mutex
func (s *SpecificationGenerator) determineExecutionSpecSeedQuantiles() (float64, float64) {
	//* Generate uniform quantiles in [0, 1).
	runQtl := s.specRand.Float64()
	memQtl := s.specRand.Float64()

	return runQtl, memQtl
}

// GenerateExecuteSpec is not thread safe as it could cause non-repeatable spec generation
func GenerateExecuteSpec(gen *rand.Rand, runQtl float64, runStats *common.FunctionRuntimeStats) (runtime int) {
	switch {
	case runQtl == 0:
		runtime = int(runStats.Percentile0)
	case runQtl <= 0.01:
		runtime = randIntBetween(gen, runStats.Percentile0, runStats.Percentile1)
	case runQtl <= 0.25:
		runtime = randIntBetween(gen, runStats.Percentile1, runStats.Percentile25)
	case runQtl <= 0.50:
		runtime = randIntBetween(gen, runStats.Percentile25, runStats.Percentile50)
	case runQtl <= 0.75:
		runtime = randIntBetween(gen, runStats.Percentile50, runStats.Percentile75)
	case runQtl <= 0.99:
		runtime = randIntBetween(gen, runStats.Percentile75, runStats.Percentile99)
	case runQtl < 1:
		runtime = randIntBetween(gen, runStats.Percentile99, runStats.Percentile100)
	}

	return runtime
}

// GenerateMemorySpec is not thread safe as it could cause non-repeatable spec generation
func GenerateMemorySpec(gen *rand.Rand, memQtl float64, memStats *common.FunctionMemoryStats) (memory int) {
	switch {
	case memQtl <= 0.01:
		memory = int(memStats.Percentile1)
	case memQtl <= 0.05:
		memory = randIntBetween(gen, memStats.Percentile1, memStats.Percentile5)
	case memQtl <= 0.25:
		memory = randIntBetween(gen, memStats.Percentile5, memStats.Percentile25)
	case memQtl <= 0.50:
		memory = randIntBetween(gen, memStats.Percentile25, memStats.Percentile50)
	case memQtl <= 0.75:
		memory = randIntBetween(gen, memStats.Percentile50, memStats.Percentile75)
	case memQtl <= 0.95:
		memory = randIntBetween(gen, memStats.Percentile75, memStats.Percentile95)
	case memQtl <= 0.99:
		memory = randIntBetween(gen, memStats.Percentile95, memStats.Percentile99)
	case memQtl < 1:
		memory = randIntBetween(gen, memStats.Percentile99, memStats.Percentile100)
	}

	return memory
}

func (s *SpecificationGenerator) generateExecutionSpecs(function *common.Function) common.RuntimeSpecification {
	runStats, memStats := function.RuntimeStats, function.MemoryStats
	if runStats.Count <= 0 || memStats.Count <= 0 {
		log.Fatal("Invalid duration or memory specification of the function '" + function.Name + "'.")
	}

	runQtl, memQtl := s.determineExecutionSpecSeedQuantiles()
	runtime := common.MinOf(common.MaxExecTimeMilli, common.MaxOf(common.MinExecTimeMilli, GenerateExecuteSpec(s.specRand, runQtl, runStats)))
	memory := common.MinOf(common.MaxMemQuotaMib, common.MaxOf(common.MinMemQuotaMib, GenerateMemorySpec(s.specRand, memQtl, memStats)))

	return common.RuntimeSpecification{
		Runtime: runtime,
		Memory:  memory,
	}
}
