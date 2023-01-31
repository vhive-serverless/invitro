package generator

import (
	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
	"math/rand"
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
func (s *SpecificationGenerator) generateIATPerGranularity(numberOfInvocations int, iatDistribution common.IatDistribution, granularity common.TraceGranularity) ([]float64, float64) {
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

	return iatResult, totalDuration
}

// GenerateIAT generates IAT according to the given distribution. Number of minutes is the length of invocationsPerMinute array
func (s *SpecificationGenerator) generateIAT(invocationsPerMinute []int, iatDistribution common.IatDistribution, granularity common.TraceGranularity) (common.IATMatrix, common.ProbabilisticDuration) {
	var IAT [][]float64
	var nonScaledDuration []float64

	numberOfMinutes := len(invocationsPerMinute)
	for i := 0; i < numberOfMinutes; i++ {
		minuteIAT, duration := s.generateIATPerGranularity(invocationsPerMinute[i], iatDistribution, granularity)

		IAT = append(IAT, minuteIAT)
		nonScaledDuration = append(nonScaledDuration, duration)
	}

	return IAT, nonScaledDuration
}

func (s *SpecificationGenerator) GenerateInvocationData(function *common.Function, iatDistribution common.IatDistribution, granularity common.TraceGranularity) *common.FunctionSpecification {
	invocationsPerMinute := function.InvocationStats.Invocations

	// Generating IAT
	iat, rawDuration := s.generateIAT(invocationsPerMinute, iatDistribution, granularity)

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
	case runQtl <= 0.95:
		runtime = s.randIntBetween(runStats.Percentile75, runStats.Percentile99)
	case runQtl <= 0.99:
		runtime = s.randIntBetween(runStats.Percentile99, runStats.Percentile100)
	case runQtl < 1:
		// NOTE: 100th percentile is smaller from the max. somehow.
		runtime = int(runStats.Percentile100)
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
