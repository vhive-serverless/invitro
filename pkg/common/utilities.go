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
	"encoding/json"
	"hash/fnv"
	"io"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"

	logger "github.com/sirupsen/logrus"
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

func GetName(function *Function) int {
	parts := strings.Split(function.Name, "-")
	if parts[0] == "test" {
		return 0
	}
	functionId, err := strconv.Atoi(parts[2])
	if err != nil {
		log.Fatal(err)
	}
	return functionId
}

// Helper function to copy files
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}

	return destFile.Sync()
}

func DeepCopy[T any](a T) (T, error) {
	var b T
	byt, err := json.Marshal(a)
	if err != nil {
		return b, err
	}
	err = json.Unmarshal(byt, &b)
	return b, err
}

func RunCommand(command string) {
	if command == "" {
		return
	}
	logger.Debug("Running command ", command)
	cmd, err := exec.Command("sh", "-c", command).Output()
	if err != nil {
		logger.Fatal(err)
	}
	logger.Debug(string(cmd))
}

func ParseLogType(logString string) string {
	logTypeArr := strings.Split(logString, "level=")
	if len(logTypeArr) > 1 {
		return strings.Split(logTypeArr[1], " ")[0]
	}
	return "info"
}

func ParseLogMessage(logString string) string {
	message := strings.Split(logString, "msg=")
	if len(message) > 1 {
		return message[1][1 : len(message[1])-1]
	}
	return logString
}
