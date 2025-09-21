package common

type Interval[T comparable] struct {
	Start T
	End   T
	Value T
}

type IntervalSearch struct {
	intervals []Interval[int]
}

func NewIntervalSearch(data []int) *IntervalSearch {
	return &IntervalSearch{
		intervals: constructIntervals(data),
	}
}

func constructIntervals(pmc []int) []Interval[int] {
	var result []Interval[int]

	sum := 0
	for i := 0; i < len(pmc); i++ {
		if pmc[i] == 0 {
			continue
		}
		startIndex := sum
		endIndex := sum + pmc[i]

		if startIndex != endIndex {
			result = append(result, Interval[int]{Start: startIndex, End: endIndex - 1, Value: i})
		}
		//logrus.Debugf("[%d, %d] = %d", startIndex, endIndex-1, i)

		sum = endIndex
	}

	return result
}

func (si *IntervalSearch) SearchInterval(val int) *Interval[int] {
	low := 0
	high := len(si.intervals) - 1

	for low <= high {
		mid := (low + high) / 2

		if val >= si.intervals[mid].Start && val <= si.intervals[mid].End {
			return &si.intervals[mid]
		} else if val < si.intervals[mid].Start {
			high = mid - 1
		} else {
			low = mid + 1
		}
	}

	return nil
}
