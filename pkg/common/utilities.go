/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package common

import (
	"hash/fnv"
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
