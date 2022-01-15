package test

import (
	"sort"
	"testing"

	fc "github.com/eth-easl/loader/internal/function"
	"github.com/stretchr/testify/assert"
)

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
	assert.Equal(t, true, isShuffled)
}
