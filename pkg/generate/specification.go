package generate

import (
	log "github.com/sirupsen/logrus"
	"math/rand"
	"os"
	"sync"
	"time"

	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
)

type SpecificationGenerator struct {
	iatRand  *rand.Rand
	iatMutex *sync.Mutex

	specRand  *rand.Rand
	specMutex *sync.Mutex
}

func NewSpecificationGenerator(seed int64) *SpecificationGenerator {
	return &SpecificationGenerator{
		iatRand:  rand.New(rand.NewSource(seed)),
		iatMutex: &sync.Mutex{},

		specRand:  rand.New(rand.NewSource(seed)),
		specMutex: &sync.Mutex{},
	}
}

//////////////////////////////////////////////////
// IAT GENERATION
//////////////////////////////////////////////////

// generateIATForAMinute generates IAT for one minute based on given number of invocations and the given distribution
func (s *SpecificationGenerator) generateIATForAMinute(numberOfInvocations int, iatDistribution IatDistribution) ([]float64, float64) {
	// TODO: missing mutex for deterministic creation of IAT for exec specs and IAT

	var iatResult []float64
	totalDuration := 0.0 // total non-scaled duration

	s.iatMutex.Lock()
	for i := 0; i < numberOfInvocations; i++ {
		var iat float64

		switch iatDistribution {
		case Exponential:
			// NOTE: Serverless in the Wild - pg. 6, paragraph 1
			iat = s.iatRand.ExpFloat64()
		case Uniform:
			iat = s.iatRand.Float64()
		case Equidistant:
			equalDistance := 60.0 * oneSecondInMicro / float64(numberOfInvocations)
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
	s.iatMutex.Unlock()

	if iatDistribution == Uniform || iatDistribution == Exponential {
		// Uniform: 		we need to scale IAT from [0, 1) to [0, 60 seconds)
		// Exponential: 	we need to scale IAT from [0, +MaxFloat64) to [0, 60 seconds)
		for i := 0; i < len(iatResult); i++ {
			// how much does the IAT contributes to the total IAT sum
			iatResult[i] = iatResult[i] / totalDuration
			// convert relative contribution to absolute on 60 second interval
			iatResult[i] = iatResult[i] * 60 * oneSecondInMicro
		}
	}

	return iatResult, totalDuration
}

// GenerateIAT generates IAT according to the given distribution. Number of minutes is the length of invocationsPerMinute array
func (s *SpecificationGenerator) GenerateIAT(invocationsPerMinute []int, iatDistribution IatDistribution) ([][]float64, []float64) {
	var IAT [][]float64
	var nonScaledDuration []float64

	numberOfMinutes := len(invocationsPerMinute)
	for i := 0; i < numberOfMinutes; i++ {
		minuteIAT, duration := s.generateIATForAMinute(invocationsPerMinute[i], iatDistribution)

		IAT = append(IAT, minuteIAT)
		nonScaledDuration = append(nonScaledDuration, duration)
	}

	return IAT, nonScaledDuration
}

//////////////////////////////////////////////////
// RUNTIME AND MEMORY GENERATION
//////////////////////////////////////////////////

// Choose a random number in between. Not thread safe.
func (s *SpecificationGenerator) randIntBetween(min, max int) int {
	if max < min {
		log.Fatal("Invalid runtime/memory specification.")
	}

	if max == min {
		return min
	} else {
		return s.specRand.Intn(max-min) + min
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
func (s *SpecificationGenerator) generateExecuteSpec(runQtl float64, runStats *tc.FunctionRuntimeStats) (runtime int) {
	switch {
	case runQtl == 0:
		runtime = runStats.Percentile0
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
		runtime = runStats.Percentile100
	}

	return runtime
}

// Should be called only when specRand is locked with its mutex
func (s *SpecificationGenerator) generateMemorySpec(memQtl float64, memStats *tc.FunctionMemoryStats) (memory int) {
	switch {
	case memQtl <= 0.01:
		memory = memStats.Percentile1
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

func (s *SpecificationGenerator) GenerateExecutionSpecs(function tc.Function) (int, int) {
	runStats, memStats := function.RuntimeStats, function.MemoryStats
	if runStats.Count <= 0 || memStats.Count <= 0 {
		log.Fatal("Invalid duration or memory specification of the function '" + function.Name + "'.")
	}

	s.specMutex.Lock()
	defer s.specMutex.Unlock()

	runQtl, memQtl := s.determineExecutionSpecSeedQuantiles()
	runtime := util.MinOf(MAX_EXEC_TIME_MILLI, util.MaxOf(MIN_EXEC_TIME_MILLI, s.generateExecuteSpec(runQtl, &runStats)))
	memory := util.MinOf(tc.MAX_MEM_QUOTA_MIB, util.MaxOf(tc.MIN_MEM_QUOTA_MIB, s.generateMemorySpec(memQtl, &memStats)))

	return runtime, memory
}

/////////////////////////////////////
// TODO: check and refactor everything below
/////////////////////////////////////

/**
 * This function waits for the waitgroup for the specified max timeout.
 * Returns true if waiting timed out.
 */
func wgWaitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
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
}

func CheckOverload(successCount, failureCount int64) bool {
	//* Amongst those returned, how many has failed?
	failureRate := float64(failureCount) / float64(successCount+failureCount)
	log.Info("Failure rate=", failureRate)
	return failureRate > OVERFLOAD_THRESHOLD
}

func DumpOverloadFlag() {
	// If the file doesn't exist, create it, or append to the file
	_, err := os.OpenFile("overload.flag", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
}
