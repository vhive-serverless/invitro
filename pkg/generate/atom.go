package generate

import (
	"math/rand"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	util "github.com/eth-easl/loader/pkg"
	fc "github.com/eth-easl/loader/pkg/function"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	STATIONARY_P_VALUE         = 0.05
	PROFILING_DURATION_MINUTES = 5  // K8s default eviction duration.
	WARMUP_DURATION_MINUTES    = 10 // Ten-minute warmup for unifying the starting time.

	OVERFLOAD_THRESHOLD = 0.3
	OVERFLOAD_TOLERANCE = 2

	FORCE_TIMEOUT_MINUTE = 0

	RPS_WARMUP_FRACTION  = 0.5 // Take th last 50% measurements.
	MAX_RPS_STARTUP_STEP = 5
)

/** Seed the math/rand package for it to be different on each run. */
// func init() {
// 	rand.Seed(time.Now().UnixNano())
// }

var iatRand *rand.Rand
var invRand *rand.Rand
var specRand *rand.Rand

func InitSeed(s int64) {
	iatRand = rand.New(rand.NewSource(s))
	invRand = rand.New(rand.NewSource(s))
	specRand = rand.New(rand.NewSource(s))
}

func GenerateOneMinuteInterarrivalTimesInMicro(invocationsPerMinute int, uniform bool) []float64 {
	oneSecondInMicro := 1000_000.0
	oneMinuteInMicro := 60*oneSecondInMicro - 1000

	rps := float64(invocationsPerMinute) / 60
	interArrivalTimes := []float64{}

	totoalDuration := 0.0
	for i := 0; i < invocationsPerMinute; i++ {
		var iat float64
		if uniform {
			iat = oneSecondInMicro / rps
		} else {
			iat = iatRand.ExpFloat64() / rps * oneSecondInMicro
		}
		//* Only guarantee microsecond-level ganularity.
		if iat < 1 {
			iat = 1
		}
		interArrivalTimes = append(interArrivalTimes, iat)
		totoalDuration += iat
	}

	if totoalDuration > oneMinuteInMicro {
		//* Normalise if it's longer than 1min.
		for i, iat := range interArrivalTimes {
			iat = iat / totoalDuration * oneMinuteInMicro
			if iat < 1 {
				iat = 1
			}
			interArrivalTimes[i] = iat
		}
	}

	// log.Info(stats.Sum(stats.LoadRawData(interArrivalTimes)))
	return interArrivalTimes
}

func CheckOverload(successCount, failureCount int64) bool {
	//* Amongst those returned, how many has failred?
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
	suffleOneMinute := func(invocations *[]int) {
		invRand.Shuffle(len(*invocations), func(i, j int) {
			(*invocations)[i], (*invocations)[j] = (*invocations)[j], (*invocations)[i]
		})
	}

	for minute := range *invocationsEachMinute {
		suffleOneMinute(&(*invocationsEachMinute)[minute])
	}
}

// Choose a random number in between.
func randIntBetween(min, max int) int {
	inverval := util.MaxOf(1, max-min)
	return specRand.Intn(inverval) + min
}

func GenerateExecutionSpecs(function tc.Function) (int, int) {

	if function.MemoryStats.Count < 0 {
		//* Custom runtime specs.
		return function.RuntimeStats.Average, function.MemoryStats.Average
	}

	var runtime, memory int
	//* Generate uniform quantiles in [0, 1).
	memQtl := specRand.Float64()
	runQtl := specRand.Float64()
	//* Generate gaussian quantiles in [0, 1).
	// sigma := .25
	// mu := .5
	// memQtl := specRand.NormFloat64()*sigma + mu
	// runQtl := specRand.NormFloat64()*sigma + mu

	runStats := function.RuntimeStats
	memStats := function.MemoryStats

	switch {
	case memQtl <= 0.01:
		memory = memStats.Percentile1
	case memQtl == 0.05:
		memory = memStats.Percentile5
	case memQtl == 0.25:
		memory = memStats.Percentile25
	case memQtl == 0.50:
		memory = memStats.Percentile50
	case memQtl == 0.75:
		memory = memStats.Percentile75
	case memQtl == 0.95:
		memory = memStats.Percentile95
	case memQtl == 0.99:
		memory = memStats.Percentile99
	case memQtl < 1:
		memory = memStats.Percentile100
	}

	switch {
	case runQtl <= 0.01:
		runtime = runStats.Percentile0
	case runQtl == 0.25:
		runtime = runStats.Percentile25
	case runQtl == 0.50:
		runtime = runStats.Percentile50
	case runQtl == 0.75:
		runtime = runStats.Percentile75
	case runQtl == 0.99:
		runtime = runStats.Percentile99
	case runQtl < 1:
		runtime = runStats.Percentile100
	}

	//* Restrict specs to valid range.
	runtime = util.MinOf(fc.MAX_EXEC_TIME_MILLI, util.MaxOf(fc.MIN_EXEC_TIME_MILLI, runtime))
	memory = util.MinOf(tc.MAX_MEM_QUOTA_MIB, util.MaxOf(1, memory))
	return runtime, memory
}

func computeActualRps(start time.Time, counter int64) float64 {
	duration := time.Since(start).Seconds()
	return float64(counter) / duration
}

func DumpOverloadFlag() {
	// If the file doesn't exist, create it, or append to the file
	_, err := os.OpenFile("overload.flag", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
}
