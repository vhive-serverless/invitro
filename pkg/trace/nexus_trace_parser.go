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
	"math/rand"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

type NexusTraceParser struct {
	DirectoryPath         string
	yamlPath              string
	duration              int
	functionNameGenerator *rand.Rand
}

func NewNexusParser(directoryPath string, totalDuration int, yamlPath string) *NexusTraceParser {
	return &NexusTraceParser{
		DirectoryPath:         directoryPath,
		yamlPath:              yamlPath,
		duration:              totalDuration,
		functionNameGenerator: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (p *NexusTraceParser) extractFunctions(invocations *[]common.FunctionInvocationStats, runtime *[]common.FunctionRuntimeStats, memory *[]common.FunctionMemoryStats) []*common.Function {
	var result []*common.Function

	for i, funcInv := range *invocations {
		funcRuntime := (*runtime)[i]
		funcMemory := (*memory)[i]

		f := &common.Function{
			Name:                fmt.Sprintf("%s-%s", funcInv.HashApp, funcInv.HashFunction),
			InvocationStats:     &funcInv,
			RuntimeStats:        &funcRuntime,
			MemoryStats:         &funcMemory,
			YAMLPath:            p.yamlPath,
			ColdStartBusyLoopMs: 0, // No memory stats for Nexus trace, so set to 0.
		}

		result = append(result, f)
	}

	return result
}

func (p *NexusTraceParser) Parse() []*common.Function {
	invocationPath := p.DirectoryPath + "/invocations.csv"

	invocationTrace := parseNexusInvocationTrace(invocationPath, p.duration)
	runtimeTrace := createMockRuntimeStats(invocationTrace)
	memoryTrace := createMockMemoryStats(invocationTrace)

	return p.extractFunctions(invocationTrace, runtimeTrace, memoryTrace)
}

func parseNexusInvocationTrace(traceFile string, traceDuration int) *[]common.FunctionInvocationStats {
	log.Infof("Parsing function invocation trace %s (duration: %d min)", traceFile, traceDuration)

	// Fit duration on (0, 1440] interval
	traceDuration = common.MaxOf(common.MinOf(traceDuration, 1440), 1)

	var result []common.FunctionInvocationStats

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to open invocation CSV file.", err)
	}
	defer csvfile.Close()

	reader := csv.NewReader(csvfile)

	// Skip header row
	_, err = reader.Read()
	if err != nil {
		if err == io.EOF {
			return &result // Empty file
		}
		log.Fatal(err)
	}
	rowID := 0
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		var invocations []int
		// Invocations start from the second column (index 1)
		for i := 1; i < 1+traceDuration; i++ {
			if i >= len(record) {
				invocations = append(invocations, 0)
				continue
			}
			num, err := strconv.Atoi(record[i])
			if err != nil {
				num = 0 // If parsing fails, assume 0 invocations
			}
			invocations = append(invocations, num)
		}

		result = append(result, common.FunctionInvocationStats{
			HashOwner:    "",
			HashApp:      record[0],
			HashFunction: strconv.Itoa(rowID),
			Trigger:      "",
			Invocations:  invocations,
		})
		rowID++
	}

	return &result
}

func createMockRuntimeStats(functions *[]common.FunctionInvocationStats) *[]common.FunctionRuntimeStats {
	var stats []common.FunctionRuntimeStats
	for _, function := range *functions {
		stats = append(stats, common.FunctionRuntimeStats{
			HashOwner:    function.HashOwner,
			HashApp:      function.HashApp,
			HashFunction: function.HashFunction,

			Average: 200, // ms
			Count:   100,
			Minimum: 100,
			Maximum: 500,

			Percentile0:   100,
			Percentile1:   120,
			Percentile25:  150,
			Percentile50:  200,
			Percentile75:  250,
			Percentile99:  400,
			Percentile100: 500,
		})
	}
	return &stats
}

func createMockMemoryStats(functions *[]common.FunctionInvocationStats) *[]common.FunctionMemoryStats {
	var stats []common.FunctionMemoryStats
	for _, function := range *functions {
		stats = append(stats, common.FunctionMemoryStats{
			HashOwner:    function.HashOwner,
			HashApp:      function.HashApp,
			HashFunction: function.HashFunction,

			Count:   100,
			Average: 128, // MB

			Percentile1:   64,
			Percentile5:   96,
			Percentile25:  112,
			Percentile50:  128,
			Percentile75:  144,
			Percentile95:  160,
			Percentile99:  192,
			Percentile100: 256,
		})
	}
	return &stats
}
