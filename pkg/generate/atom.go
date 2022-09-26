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

const (
	// 1ms (min. billing unit of AWS)
	MIN_EXEC_TIME_MILLI = 1
	// 60s (avg. p96 from Wild)
	MAX_EXEC_TIME_MILLI = 60e3
)

const (
	// The stationary p-value for the ADF test that warns users if the cluster hasn't been warmed up
	// after predefined period.
	STATIONARY_P_VALUE = 0.05
	// K8s default eviction duration, after which all decisions made before should either be executed
	// or failed (and cleaned).
	PROFILING_DURATION_MINUTES = 5
	// Ten-minute warmup for unifying the starting time when the experiments consists of multiple runs.
	WARMUP_DURATION_MINUTES = 10
	// The fraction of RETURNED failures to the total invocations fired. This threshold is a patent overestimation
	// and it's here to stop the sweeping when the cluster is no longer functioning.
	OVERFLOAD_THRESHOLD = 0.3
	// The number of times allowed for the measured failure rate to surpass the `OVERFLOAD_THRESHOLD`.
	// It's here to avoid "early stopping" so that we make sure sufficient load has been imposed on the system.
	OVERFLOAD_TOLERANCE = 2
	// The compulsory timeout after which the loader will no longer await the goroutines that haven't returned,
	// and move on to the next generation round. We need it because some functions may end up in nowhere and never return.
	// By default, the wait-group will halt forever in that case.
	FORCE_TIMEOUT_MINUTE = 15
	// The portion of measurements we take in the RPS mode. The first 20% serves as a step-wise warm-up, and
	// we only take the last 80% of the measurements.
	RPS_WARMUP_FRACTION = 0.2
	// The maximum step size in the early stage of the RPS mode -- we shouldn't take too large a RPS step before reaching
	// ~100RPS in order to ensure sufficient number of measurements for lower variance (smaller the RPS, the less total data points).
	MAX_RPS_STARTUP_STEP = 5
)

/** Seed the math/rand package for it to be different on each run. */
// func init() {
// 	rand.Seed(time.Now().UnixNano())
// }

var iatRand *rand.Rand
var iatMutex *sync.Mutex

var invRand *rand.Rand

var specRand *rand.Rand
var specMutex *sync.Mutex

type IatDistribution int

const (
	Exponential IatDistribution = iota
	Uniform     IatDistribution = iota
	Equidistant IatDistribution = iota
)

const (
	oneSecondInMicro = 1_000_000.0
)

func InitSeed(s int64) {
	iatRand = rand.New(rand.NewSource(s))
	iatMutex = &sync.Mutex{}

	// TODO: check this
	invRand = rand.New(rand.NewSource(s))

	specRand = rand.New(rand.NewSource(s))
	specMutex = &sync.Mutex{}
}

//////////////////////////////////////////////////
// IAT GENERATION
//////////////////////////////////////////////////

// generateIATForAMinute generates IAT for one minute based on given number of invocations and the given distribution
func generateIATForAMinute(numberOfInvocations int, iatDistribution IatDistribution) []float64 {
	//* Launching goroutine takes time, especially under high load (5%: e.g., 3s/minute),
	//* so we need to guarantee the required #invocations before timeout.
	/*slackFrac := .05
	if iatDistribution == Exponential {
		slackFrac *= 2 // Empirically, we need more slack when generating exponential due to potentially shorter intervals.
	}
	slackTimeMicro := float64(totalDurationInSec) * slackFrac * oneSecondInMicro*/
	//durationInMicro := float64(numberOfInvocations) * oneSecondInMicro - slackTimeMicro

	// TODO: do we want to keep the slack?
	// TODO: missing mutex for deterministic creation of IAT for exec specs and IAT

	var iatResult []float64
	totalDuration := 0.0

	// equal distance
	equalDistance := oneSecondInMicro / float64(numberOfInvocations)

	iatMutex.Lock()
	for i := 0; i < numberOfInvocations; i++ {
		var iat float64

		switch iatDistribution {
		case Exponential:
			// TODO: check out this
			rps := float64(numberOfInvocations) / 60
			iat = iatRand.ExpFloat64() / rps * oneSecondInMicro
		case Uniform:
			// TODO: do we want to cut resize equalDistance
			iat = iatRand.Float64() * equalDistance
		case Equidistant:
			iat = equalDistance
		default:
			log.Fatal("Unsupported IAT distribution.")
		}

		if iat < 1 {
			// No nanoseconds-level granularity, only microsecond
			iat = 1
		}

		iatResult = append(iatResult, iat)
		totalDuration += iat
	}
	iatMutex.Unlock()

	//////////////////////////////
	// START OF UNKNOWN
	// TODO: figure out purpose of this
	if iatDistribution == Uniform {
		tmp := []float64{iatResult[0]}
		for i, iat := range iatResult[1:] {
			i++
			gap := equalDistance - iatResult[i-1] // Fill in the remaining time of previous iat in its equalDistance.

			if gap < 0 {
				log.Info(equalDistance, iatResult[i-1])
			}
			tmp = append(tmp, gap+iat)
		}
		iatResult = tmp
	}
	// END OF UNKNOWN
	//////////////////////////////

	if totalDuration > oneSecondInMicro {
		// If all the generated invocations do not fit within a single minute normalize them
		for i, iat := range iatResult {
			iat /= totalDuration * oneSecondInMicro
			if iat < 1 {
				// No nanoseconds-level granularity, only microsecond
				iat = 1
			}

			iatResult[i] = iat
		}
	}

	return iatResult
}

// GenerateIAT generates IAT according to the given distribution. Number of minutes is the length of invocationsPerMinute array
func GenerateIAT(invocationsPerMinute []int, iatDistribution IatDistribution) []float64 {
	var result []float64

	numberOfMinutes := len(invocationsPerMinute)
	for i := 0; i < numberOfMinutes; i++ {
		minuteIAT := generateIATForAMinute(invocationsPerMinute[i], iatDistribution)
		result = append(result, minuteIAT...)
	}

	return result
}

//////////////////////////////////////////////////
// RUNTIME AND MEMORY GENERATION
//////////////////////////////////////////////////

// Choose a random number in between. Not thread safe.
func randIntBetween(min, max int) int {
	if max < min {
		panic("Invalid runtime/memory specification.")
	} else if max == min {
		return min
	} else {
		return specRand.Intn(max-min) + min
	}
}

// Should be called only when specRand is locked with its mutex
func determineExecutionSpecSeedQuantiles() (float64, float64) {
	//* Generate uniform quantiles in [0, 1).
	runQtl := specRand.Float64()
	memQtl := specRand.Float64()

	return runQtl, memQtl
}

// Should be called only when specRand is locked with its mutex
func generateExecuteSpec(runQtl float64, runStats *tc.FunctionRuntimeStats) (runtime int) {
	switch {
	case runQtl == 0:
		runtime = runStats.Percentile0
	case runQtl <= 0.01:
		runtime = randIntBetween(runStats.Percentile0, runStats.Percentile1)
	case runQtl <= 0.25:
		runtime = randIntBetween(runStats.Percentile1, runStats.Percentile25)
	case runQtl <= 0.50:
		runtime = randIntBetween(runStats.Percentile25, runStats.Percentile50)
	case runQtl <= 0.75:
		runtime = randIntBetween(runStats.Percentile50, runStats.Percentile75)
	case runQtl <= 0.95:
		runtime = randIntBetween(runStats.Percentile75, runStats.Percentile99)
	case runQtl <= 0.99:
		runtime = randIntBetween(runStats.Percentile99, runStats.Percentile100)
	case runQtl < 1:
		// NOTE: 100th percentile is smaller from the max. somehow.
		runtime = runStats.Percentile100
	}

	return runtime
}

// Should be called only when specRand is locked with its mutex
func generateMemorySpec(memQtl float64, memStats *tc.FunctionMemoryStats) (memory int) {
	switch {
	case memQtl <= 0.01:
		memory = memStats.Percentile1
	case memQtl <= 0.05:
		memory = randIntBetween(memStats.Percentile1, memStats.Percentile5)
	case memQtl <= 0.25:
		memory = randIntBetween(memStats.Percentile5, memStats.Percentile25)
	case memQtl <= 0.50:
		memory = randIntBetween(memStats.Percentile25, memStats.Percentile50)
	case memQtl <= 0.75:
		memory = randIntBetween(memStats.Percentile50, memStats.Percentile75)
	case memQtl <= 0.95:
		memory = randIntBetween(memStats.Percentile75, memStats.Percentile95)
	case memQtl <= 0.99:
		memory = randIntBetween(memStats.Percentile95, memStats.Percentile99)
	case memQtl < 1:
		memory = randIntBetween(memStats.Percentile99, memStats.Percentile100)
	}

	return memory
}

func GenerateExecutionSpecs(function tc.Function) (int, int) {
	runStats, memStats := function.RuntimeStats, function.MemoryStats
	if runStats.Count <= 0 || memStats.Count <= 0 {
		panic("Invalid duration or memory specification of the function '" + function.Name + "'.")
	}

	specMutex.Lock()
	defer specMutex.Unlock()

	runQtl, memQtl := determineExecutionSpecSeedQuantiles()
	runtime := util.MinOf(MAX_EXEC_TIME_MILLI, util.MaxOf(MIN_EXEC_TIME_MILLI, generateExecuteSpec(runQtl, &runStats)))
	memory := util.MinOf(tc.MAX_MEM_QUOTA_MIB, util.MaxOf(tc.MIN_MEM_QUOTA_MIB, generateMemorySpec(memQtl, &memStats)))

	return runtime, memory
}

/////////////////////////////////////
// TODO: check and refactor everything below
/////////////////////////////////////

func CheckOverload(successCount, failureCount int64) bool {
	//* Amongst those returned, how many has failed?
	failureRate := float64(failureCount) / float64(successCount+failureCount)
	log.Info("Failure rate=", failureRate)
	return failureRate > OVERFLOAD_THRESHOLD
}

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

/**
 * This function has/uses side-effects, but for the sake of performance
 * keep it for now.
 */
func ShuffleAllInvocationsInplace(invocationsEachMinute *[][]int) {
	// TODO: may be missing mutex

	suffleOneMinute := func(invocations *[]int) {
		invRand.Shuffle(len(*invocations), func(i, j int) {
			(*invocations)[i], (*invocations)[j] = (*invocations)[j], (*invocations)[i]
		})
	}

	for minute := range *invocationsEachMinute {
		suffleOneMinute(&(*invocationsEachMinute)[minute])
	}
}

func DumpOverloadFlag() {
	// If the file doesn't exist, create it, or append to the file
	_, err := os.OpenFile("overload.flag", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
}
