package test

import (
	"sort"
	"testing"

	fc "github.com/eth-easl/loader/internal/function"
	"github.com/montanaflynn/stats"
	"github.com/stretchr/testify/assert"
)

func TestGenerateIat(t *testing.T) {
	invocationsPerMinute := 90_000
	iats := fc.GenerateInterarrivalTimesInMilli(invocationsPerMinute)
	duration, _ := stats.Sum(stats.LoadRawData(iats))

	assert.Equal(t, invocationsPerMinute, len(iats))
	assert.Greater(t, 60_000.0, duration)
}

func TestShuffling(t *testing.T) {
	arr := [][]int{}
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})
	arr = append(arr, []int{1, 1, 2, 2, 3, 3})

	fc.ShuffleAllInvocationsInplace(&arr)

	isShuffled := false
	for idx := range arr {
		a := arr[idx]
		isShuffled = !sort.SliceIsSorted(a, func(i, j int) bool {
			return a[i] < a[j]
		})
	}
	assert.True(t, isShuffled)
}
