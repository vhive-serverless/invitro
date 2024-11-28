package common

import "testing"

func TestIntervalTree(t *testing.T) {
	pmc := []int{0, 10, 10, 10, 10, 10, 0, 0, 2}
	intervals := NewIntervalSearch(pmc)

	pairs := map[int]int{
		// minute 1
		0: 1,
		5: 1,
		9: 1,
		// minute 2
		10: 2,
		17: 2,
		19: 2,
		// minute 5
		40: 5,
		45: 5,
		49: 5,
		// minute 8
		50: 8,
		51: 8,
		52: -1, // non-existing
	}

	for iatIndex, minute := range pairs {
		got := intervals.SearchInterval(iatIndex)

		if got == nil && minute != -1 {
			t.Errorf("Value %d not found ", iatIndex)
		} else if got != nil {
			if minute != got.Value {
				t.Errorf("Unexpected value received - iatIndex: %d; expected: %d; got: %d", iatIndex, minute, got.Value)
			}
		}
	}
}
