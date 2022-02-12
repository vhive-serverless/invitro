package util

import (
	"math/rand"
	"time"
)

/** Seed the math/rand package for it to be different on each run. */
func init() {
	rand.Seed(time.Now().UnixNano())
}

func RandIntBetween(min, max int) int {
	return rand.Intn(max-min) + min
}

func RandBool() bool {
	return rand.Int31()&0x01 == 0
}

func B2Kib(numB uint32) uint32 {
	return numB / 1024
}

func Mib2b(numMb uint32) uint32 {
	return numMb * 1024 * 1024
}

func Mib2Kib(numMb uint32) uint32 {
	return numMb * 1024
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

func MaxOf(vars ...int) int {
	max := vars[0]

	for _, i := range vars {
		if max < i {
			max = i
		}
	}

	return max
}

func Check(e error) {
	if e != nil {
		panic(e)
	}
}

// func Hash(s string) uint32 {
// 	h := fnv.New32a()
// 	h.Write([]byte(s))
// 	return h.Sum32()
// }
