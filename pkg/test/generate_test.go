package test

import (
	"math/rand"
	"sort"
	"sync"
	"testing"

	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func init() {
	gen.InitSeed(42)
}

func TestGenCheckOverload(t *testing.T) {
	successCount := 100
	assert.False(t, gen.CheckOverload(int64(successCount), int64(successCount/10)))
	assert.True(t, gen.CheckOverload(int64(successCount), int64(3*successCount)))
}

func TestGenerateIat(t *testing.T) {
	invocationsPerMinute := 30_000
	oneMinuteInMicro := 60_000_000.0

	/** Testing Equidistance */
	iats := gen.GenerateOneMinuteInterarrivalTimesInMicro(invocationsPerMinute, gen.Equidistant)
	duration, _ := stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, iats[rand.Intn(len(iats))], iats[rand.Intn(len(iats))])
	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, oneMinuteInMicro, duration)
	t.Log(duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}

	/** Testing Poisson */
	iats = gen.GenerateOneMinuteInterarrivalTimesInMicro(invocationsPerMinute, gen.Poisson)
	duration, _ = stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, oneMinuteInMicro, duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}

	iats = gen.GenerateOneMinuteInterarrivalTimesInMicro(0, gen.Poisson)
	assert.Equal(t, 0, len(iats))
	// t.Log(iats)
}

func TestShuffling(t *testing.T) {
	arr := [][]int{}
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})

	gen.ShuffleAllInvocationsInplace(&arr)

	isShuffled := false
	for idx := range arr {
		a := arr[idx]
		isShuffled = !sort.SliceIsSorted(a, func(i, j int) bool {
			return a[i] < a[j]
		})
		log.Info(arr)
	}
	assert.True(t, isShuffled)
}

func TestGenerateExecutionSpecsDataRace(t *testing.T) {
	fakeFunction := tc.Function{
		RuntimeStats: tc.FunctionRuntimeStats{
			Average:       50,
			Minimum:       0,
			Maximum:       100,
			Percentile0:   0,
			Percentile1:   1,
			Percentile25:  25,
			Percentile50:  50,
			Percentile75:  75,
			Percentile99:  99,
			Percentile100: 100,
		},
		MemoryStats: tc.FunctionMemoryStats{
			Average:       50,
			Percentile1:   1,
			Percentile5:   5,
			Percentile25:  25,
			Percentile50:  50,
			Percentile75:  75,
			Percentile95:  95,
			Percentile99:  99,
			Percentile100: 100,
		},
	}
	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			gen.GenerateExecutionSpecs(fakeFunction)
			wg.Done()
		}()
	}
	wg.Wait()
}
