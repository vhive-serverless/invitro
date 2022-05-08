package test

import (
	"math/rand"
	"sort"
	"testing"

	gen "github.com/eth-easl/loader/pkg/generate"
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

	invocationsPerMinute := 20_000
	iats := gen.GenerateInterarrivalTimesInMicro(invocationsPerMinute, true)
	duration, _ := stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, iats[rand.Intn(len(iats))], iats[rand.Intn(len(iats))])
	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, 60_000_000.0, duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}

	iats = gen.GenerateInterarrivalTimesInMicro(invocationsPerMinute, false)
	duration, _ = stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, 60_000_000.0, duration)

	for _, iat := range iats {
		assert.GreaterOrEqual(t, iat, 1.0)
	}

	iats = gen.GenerateInterarrivalTimesInMicro(0, false)
	assert.Equal(t, 0, len(iats))
	t.Log(iats)
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
