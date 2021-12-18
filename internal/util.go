package util

import (
	"math/rand"
	"time"
)

/** Seed the math/rand package for it to be different on each run. */
func init() {
	rand.Seed(time.Now().UnixNano())
}

func GetRandBool() bool {
	return rand.Int31()&0x01 == 0
}

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
