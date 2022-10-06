package common

import (
	"hash/fnv"
	"io/ioutil"
	"log"
	"math/rand"
	"strconv"
	"strings"
)

type Pair struct {
	Key   interface{}
	Value int
}
type PairList []Pair

func (p PairList) Len() int {
	return len(p)
}
func (p PairList) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}
func (p PairList) Less(i, j int) bool {
	return p[i].Value < p[j].Value
}

func ReadIntArray(filePath, delimiter string) []int {
	content, err := ioutil.ReadFile(filePath)
	Check(err)
	lines := strings.Split(string(content), delimiter)
	return SliceAtoi(lines)
}

func SliceAtoi(sa []string) []int {
	si := make([]int, 0, len(sa))
	for _, a := range sa {
		i, err := strconv.Atoi(a)
		Check(err)
		si = append(si, i)
	}
	return si
}

func Hex2Int(hexStr string) int64 {
	// remove 0x suffix if found in the input string
	cleaned := strings.Replace(hexStr, "0x", "", -1)

	// base 16 for hexadecimal
	result, _ := strconv.ParseUint(cleaned, 16, 64)
	return int64(result)
}

func RandIntBetween(min, max int) int {
	inverval := MaxOf(1, max-min)
	return rand.Intn(inverval) + min
}

func RandBool() bool {
	return rand.Int31()&0x01 == 0
}

func B2Kib(numB uint32) uint32 {
	return numB / 1024
}

func Kib2Mib(numB uint32) uint32 {
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
		log.Fatal(e)
	}
}

func Hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func SumNumberOfInvocations(withWarmup bool, totalDuration int, functions []*Function) int {
	result := 0

	for _, f := range functions {
		minuteIndex := 0
		if withWarmup {
			// ignore the first minute of the trace if warmup is enabled
			minuteIndex = 1
		}

		for ; minuteIndex < totalDuration; minuteIndex++ {
			result += f.InvocationStats.Invocations[minuteIndex]
		}
	}

	return result
}
