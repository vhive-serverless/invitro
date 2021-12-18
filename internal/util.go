package util

func MinOf(vars ...int) int {
	min := vars[0]

	for _, i := range vars {
		if min > i {
			min = i
		}
	}

	return min
}

func Check(e error) {
	if e != nil {
		panic(e)
	}
}
