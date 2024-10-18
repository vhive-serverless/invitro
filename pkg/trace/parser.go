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
	"github.com/gocarina/gocsv"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/generator"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

type AzureTraceParser struct {
	DirectoryPath string
	YAMLPath      string

	duration              int
	functionNameGenerator *rand.Rand
}

func NewAzureParser(directoryPath string, yamlPath string, totalDuration int) *AzureTraceParser {
	return &AzureTraceParser{
		DirectoryPath: directoryPath,
		YAMLPath:      yamlPath,

		duration:              totalDuration,
		functionNameGenerator: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func createRuntimeMap(runtime *[]common.FunctionRuntimeStats) map[string]*common.FunctionRuntimeStats {
	result := make(map[string]*common.FunctionRuntimeStats)

	for i := 0; i < len(*runtime); i++ {
		result[(*runtime)[i].HashFunction] = &(*runtime)[i]
	}

	return result
}

func createMemoryMap(runtime *[]common.FunctionMemoryStats) map[string]*common.FunctionMemoryStats {
	result := make(map[string]*common.FunctionMemoryStats)

	for i := 0; i < len(*runtime); i++ {
		result[(*runtime)[i].HashFunction] = &(*runtime)[i]
	}

	return result
}

func createDirigentMetadataMap(metadata *[]common.DirigentMetadata) map[string]*common.DirigentMetadata {
	result := make(map[string]*common.DirigentMetadata)

	for i := 0; i < len(*metadata); i++ {
		result[(*metadata)[i].HashFunction] = &(*metadata)[i]
	}

	return result
}

func (p *AzureTraceParser) extractFunctions(invocations *[]common.FunctionInvocationStats,
	runtime *[]common.FunctionRuntimeStats, memory *[]common.FunctionMemoryStats,
	dirigentMetadata *[]common.DirigentMetadata, platform string) []*common.Function {

	var result []*common.Function

	runtimeByHashFunction := createRuntimeMap(runtime)
	memoryByHashFunction := createMemoryMap(memory)

	var dirigentMetadataByHashFunction map[string]*common.DirigentMetadata
	if dirigentMetadata != nil {
		dirigentMetadataByHashFunction = createDirigentMetadataMap(dirigentMetadata)
	}

	gen := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := 0; i < len(*invocations); i++ {
		invocationStats := (*invocations)[i]

		function := &common.Function{
			Name: fmt.Sprintf("%s-%d", "t"+invocationStats.HashFunction[0:18], rand.Uint64()),

			InvocationStats: &invocationStats,
			RuntimeStats:    runtimeByHashFunction[invocationStats.HashFunction],
			MemoryStats:     memoryByHashFunction[invocationStats.HashFunction],

			ColdStartBusyLoopMs: generator.ComputeBusyLoopPeriod(generator.GenerateMemorySpec(gen, gen.Float64(), memoryByHashFunction[invocationStats.HashFunction])),
		}

		if dirigentMetadata != nil {
			function.DirigentMetadata = dirigentMetadataByHashFunction[invocationStats.HashFunction]
		} else if strings.Contains(strings.ToLower(platform), "knative") {
			// values are not used for Knative so they are irrelevant
			function.DirigentMetadata = convertKnativeYamlToDirigentMetadata(p.YAMLPath)
		}

		result = append(result, function)
	}

	return result
}

func (p *AzureTraceParser) Parse(platform string) []*common.Function {
	invocationPath := p.DirectoryPath + "/invocations.csv"
	runtimePath := p.DirectoryPath + "/durations.csv"
	memoryPath := p.DirectoryPath + "/memory.csv"
	dirigentPath := p.DirectoryPath + "/dirigent.csv"

	invocationTrace := parseInvocationTrace(invocationPath, p.duration)
	runtimeTrace := parseRuntimeTrace(runtimePath)
	memoryTrace := parseMemoryTrace(memoryPath)
	dirigentMetadata := parseDirigentMetadata(dirigentPath, platform)

	return p.extractFunctions(invocationTrace, runtimeTrace, memoryTrace, dirigentMetadata, platform)
}

func parseInvocationTrace(traceFile string, traceDuration int) *[]common.FunctionInvocationStats {
	log.Infof("Parsing function invocation trace %s (duration: %d min)", traceFile, traceDuration)

	// Fit duration on (0, 1440] interval
	traceDuration = common.MaxOf(common.MinOf(traceDuration, 1440), 1)

	var result []common.FunctionInvocationStats

	invocationIndices := make([][]int, traceDuration)
	totalInvocations := make([]int, traceDuration)

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to open invocation CSV file.", err)
	}

	reader := csv.NewReader(csvfile)

	rowID := -1
	hashOwnerIndex, hashAppIndex, hashFunctionIndex, invocationColumnIndex := -1, -1, -1, -1

	for {
		record, err := reader.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if rowID == -1 {
			// Parse header
			for i := 0; i < 4; i++ {
				switch strings.ToLower(record[i]) {
				case "hashowner":
					hashOwnerIndex = i
				case "hashapp":
					hashAppIndex = i
				case "hashfunction":
					hashFunctionIndex = i
				case "trigger": //! Unused field.
					invocationColumnIndex = i + 1
				}
			}

			if hashOwnerIndex == -1 || hashAppIndex == -1 || hashFunctionIndex == -1 {
				log.Fatal("Invocation trace does not contain at least one of the hashes.")
			}

			if invocationColumnIndex == -1 {
				invocationColumnIndex = 3
			}
		} else {
			// Parse invocations
			var invocations []int

			for i := invocationColumnIndex; i < invocationColumnIndex+traceDuration; i++ {
				minute := i - invocationColumnIndex
				num, err := strconv.Atoi(record[i])
				common.Check(err)

				invocations = append(invocations, num)

				for j := 0; j < num; j++ {
					invocationIndices[minute] = append(invocationIndices[minute], rowID)
				}
				totalInvocations[minute] = totalInvocations[minute] + num
			}

			result = append(result, common.FunctionInvocationStats{
				HashOwner:    record[hashOwnerIndex],
				HashApp:      record[hashAppIndex],
				HashFunction: record[hashFunctionIndex],
				Trigger:      record[invocationColumnIndex-1],
				Invocations:  invocations,
			})
		}

		rowID++
	}

	return &result
}

func parseRuntimeTrace(traceFile string) *[]common.FunctionRuntimeStats {
	log.Infof("Parsing function duration trace: %s\n", traceFile)

	f, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to open trace runtime specification file.")
	}
	defer f.Close()

	var runtime []common.FunctionRuntimeStats
	err = gocsv.UnmarshalFile(f, &runtime)
	if err != nil {
		log.Fatal("Failed to parse trace runtime specification.")
	}

	return &runtime
}

func parseMemoryTrace(traceFile string) *[]common.FunctionMemoryStats {
	log.Infof("Parsing function memory trace: %s", traceFile)

	f, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to open trace memory specification file.")
	}
	defer f.Close()

	var memory []common.FunctionMemoryStats
	err = gocsv.UnmarshalFile(f, &memory)
	if err != nil {
		log.Fatal("Failed to parse trace runtime specification.")
	}

	return &memory
}

func parseDirigentMetadata(traceFile string, platform string) *[]common.DirigentMetadata {
	if platform != "Dirigent" {
		return nil
	}

	log.Infof("Parsing Dirigent metadata: %s", traceFile)

	f, err := os.Open(traceFile)
	if err != nil {
		log.Error("Failed to open trace memory specification file.")
		return nil
	}
	defer f.Close()

	var metadata []common.DirigentMetadata
	err = gocsv.UnmarshalFile(f, &metadata)
	if err != nil {
		log.Fatal("Failed to parse trace runtime specification.")
	}

	return &metadata
}
