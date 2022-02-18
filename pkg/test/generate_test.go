package test

import (
	"math/rand"
	"sort"
	"testing"

	gen "github.com/eth-easl/loader/pkg/generate"
	"github.com/montanaflynn/stats"
	"github.com/stretchr/testify/assert"
)

func TestGenerateIat(t *testing.T) {
	invocationsPerMinute := 20_000
	iats := gen.GenerateInterarrivalTimesInMicro(0, invocationsPerMinute, true)
	duration, _ := stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, iats[rand.Intn(len(iats))], iats[rand.Intn(len(iats))])
	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, 60_000_000.0, duration)

	iats = gen.GenerateInterarrivalTimesInMicro(0, invocationsPerMinute, false)
	duration, _ = stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, 60_000_000.0, duration)
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
	}
	assert.True(t, isShuffled)
}
