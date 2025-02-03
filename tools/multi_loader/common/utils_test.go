package common

import (
	"testing"
)

func TestNextProduct(t *testing.T) {
	ints := []int{3, 1, 2}
	nextProduct := NextCProduct(ints)
	expectedArrs := [][]int{
		{0, 0, 0},
		{0, 0, 1},
		{0, 0, 2},
		{0, 1, 0},
		{0, 1, 1},
		{0, 1, 2},
		{1, 0, 0},
		{1, 0, 1},
		{1, 0, 2},
		{1, 1, 0},
		{1, 1, 1},
		{1, 1, 2},
		{2, 0, 0},
		{2, 0, 1},
		{2, 0, 2},
		{2, 1, 0},
		{2, 1, 1},
		{2, 1, 2},
		{3, 0, 0},
		{3, 0, 1},
		{3, 0, 2},
		{3, 1, 0},
		{3, 1, 1},
		{3, 1, 2},
	}
	curI := 0

	for {
		product := nextProduct()
		if len(product) == 0 {
			if curI != len(expectedArrs) {
				t.Fatalf("Expected %d products, got %d", len(expectedArrs), curI)
			}
			break
		}
		if len(product) != len(ints) {
			t.Fatalf("Expected product length %d, got %d", len(ints), len(product))
		}
		for i, v := range product {
			if v != expectedArrs[curI][i] {
				t.Fatalf("Expected %v, got %v", expectedArrs[curI], product)
			}
		}
		curI++
	}
}
