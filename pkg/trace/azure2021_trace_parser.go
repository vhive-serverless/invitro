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

package trace

import (
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/generator"

	log "github.com/sirupsen/logrus"
)

type Azure2021TraceParser struct {
	FilePath              string // File path to Azure2021 CSV
	dirigentYamlPath      string
	durationMinutes       int
	functionNameGenerator *rand.Rand // TODO: remove, seems useless
}

func NewAzure2021Parser(filePath string, totalMinutesToParse int, dirigentYamlPath string) *Azure2021TraceParser {
	return &Azure2021TraceParser{
		FilePath:              filePath,
		dirigentYamlPath:      dirigentYamlPath,
		durationMinutes:       totalMinutesToParse,
		functionNameGenerator: rand.New(rand.NewSource(time.Now().UnixNano())), //TODO: remove, seems useless
	}
}

func (p *Azure2021TraceParser) Parse() []*common.Function {

	invocationTracker := ParseCSVFile(p.FilePath)

	var functions []*common.Function

	/* invocationTracker populated, begin creating function array. */
	for funcID, invocationSlice := range invocationTracker {

		funcSpec, empty := GenerateFunctionSpecification(funcID, invocationSlice, p.durationMinutes)
		if empty {
			continue
		}

		// Normally directly lifted from csv file.
		memoryStats := common.FunctionMemoryStats{Percentile100: 300}

		// TODO: aaaaaaa
		function := common.Function{
			Name:                fmt.Sprintf("%s-%.5s-%.5s-%d", common.FunctionNamePrefix, funcID.appHash, funcID.functionHash, p.functionNameGenerator.Uint64()),
			YAMLPath:            p.dirigentYamlPath,
			ColdStartBusyLoopMs: generator.ComputeBusyLoopPeriod(150),
			MemoryStats:         &memoryStats,
		}

		function.Specification = funcSpec

		functions = append(functions, &function)
	}

	return functions
}

/* Function Invocation Table */
type UniqueFunctionID struct {
	appHash, functionHash string
}
type Invocation struct {
	startTime float64 // Seconds
	duration  float64 // Seconds
}
type Invocations []Invocation

// Reads csv, reads all fields, calculates "start_timestamp". Returns data as a hashmap.
func ParseCSVFile(filePath string) map[UniqueFunctionID]Invocations {

	// Open File Reader
	fd, err := os.Open(filePath)
	if err != nil {
		log.Fatal("Failed to open Azure 2021 CSV file.", err)
	}
	reader := csv.NewReader(fd)

	invocationTracker := make(map[UniqueFunctionID]Invocations) //TODO, provide a capacity hint after finished implementation.

	rowID := -1
	hashAppIndex, hashFunctionIndex, endTimestampIndex, durationIndex := -1, -1, -1, -1

	/* Parse csv data into array of invocation time + duration, for each function ID */
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		// Parse column headers
		if rowID == -1 {
			for i := 0; i < 4; i++ {
				switch strings.ToLower(record[i]) {
				case "app":
					hashAppIndex = i
				case "func":
					hashFunctionIndex = i
				case "end_timestamp":
					endTimestampIndex = i
				case "duration":
					durationIndex = i
				}
			}
			if hashAppIndex == -1 || hashFunctionIndex == -1 || endTimestampIndex == -1 || durationIndex == -1 {
				log.Fatal("Azure2021 trace file has missing columns")
			}

		} else {
			// Parse data row
			appHash := record[hashAppIndex]
			functionHash := record[hashFunctionIndex]
			funcID := UniqueFunctionID{appHash, functionHash}

			endTimestamp, err1 := strconv.ParseFloat(record[endTimestampIndex], 64)
			duration, err2 := strconv.ParseFloat(record[durationIndex], 64)
			if err1 != nil || err2 != nil {
				log.Fatal("Error during string to float64 conversion:", err1, err2)
			}

			startTimestamp := endTimestamp - duration
			invocation := Invocation{startTimestamp, duration}

			invocationTracker[funcID] = append(invocationTracker[funcID], invocation)
		}
		rowID++
	}

	return invocationTracker
}

func GenerateFunctionSpecification(funcID UniqueFunctionID, invocationSlice Invocations, durationMinutes int) (*common.FunctionSpecification, bool) {

	// sort from first to last invocation
	sort.Slice(invocationSlice, func(i, j int) bool {
		return invocationSlice[i].startTime < invocationSlice[j].startTime
	})

	// Generate IAT Array (IATs are microsecond precision) (cannot be same microsecond)
	IATArray := common.IATArray{}
	var runtimeArray common.RuntimeSpecificationArray

	finalInvocation := invocationSlice[len(invocationSlice)-1].startTime
	lastMinuteNumber := int(time.Duration(finalInvocation * float64(time.Second)).Minutes())
	perMinuteCount := make([]int, lastMinuteNumber+1)

	var previousInvocationTimestamp float64 = 0.0

	for _, invocation := range invocationSlice {
		// truncate if function invocation ends after durationMinutes.
		if (invocation.startTime + invocation.duration) > float64(durationMinutes*60) {
			continue
		}

		invocation_microseconds := invocation.startTime * 1_000_000 //TODO consider math rounding errors.
		var iat float64
		if len(IATArray) == 0 {
			iat = invocation_microseconds
			previousInvocationTimestamp = invocation_microseconds
		} else {
			iat = invocation_microseconds - previousInvocationTimestamp
			previousInvocationTimestamp = invocation_microseconds
		}
		// Negative iat check
		if iat < 0 {
			log.Fatalf("Encountered negative iat of %s", strconv.FormatFloat(iat, 'f', -1, 64))
		}

		//

		IATArray = append(IATArray, iat)

		duration := time.Duration(invocation.startTime * float64(time.Second))
		minutesPassed := int(duration.Minutes())
		perMinuteCount[minutesPassed]++

		runtime_milliseconds := int(math.Round(invocation.duration * 1_000))
		memory := 150 // TODO, allow user to specify memory, current value was visual inspecction of average
		runtimeArray = append(runtimeArray, common.RuntimeSpecification{Runtime: runtime_milliseconds, Memory: memory})
	}

	// If all invocations truncated out
	if len(IATArray) == 0 {
		return nil, true
	}

	funcSpec := &common.FunctionSpecification{
		IAT:                  IATArray,
		PerMinuteCount:       perMinuteCount,
		RuntimeSpecification: runtimeArray,
	}

	return funcSpec, false
}
