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
	"math"
	"testing"

	"encoding/json"
	"os"
	"strconv"
	"strings"

	"reflect"
	"sort"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

// Test full parser usage, verifying values of aaaaa-11111 function
func TestAzure2021ParserWrapper(t *testing.T) {
	tracePath := "test_data/Azure2021/Azure2021_30.csv"
	durationToParse := 3
	yamlPath := "dummy"
	writeToFile := false

	traceParser := NewAzure2021Parser(tracePath, durationToParse, yamlPath)
	functions := traceParser.Parse()

	if len(functions) != 14 {
		t.Errorf("Invalid function array length. Expected 14, Got %d", len(functions))
	}

	substr := "trace-func-aaaaa-11111-"
	for _, function := range functions {
		str := function.Name
		if strings.Contains(str, substr) {
			tolerance := 0.00001
			if math.Abs(function.Specification.IAT[0]-1000000) > tolerance {
				t.Errorf("Expected IAT value of 1000000, Got %f", function.Specification.IAT[0])
			} else if function.Specification.RuntimeSpecification[0].Runtime != 9000 {
				t.Errorf("Expected Runtime value of 9000, Got %d", function.Specification.RuntimeSpecification[0].Runtime)
			}
		}
	}

	if writeToFile {
		ReadOrWriteSpecificationToFile(functions, true, false)
	}
}

// Test data read and derive "start_timestamp" into InvocationTracker hashmap
func TestAzure2021ParseCSVFile(t *testing.T) {
	var filePath string = "test_data/Azure2021/Azure2021_30.csv"
	invocationTracker := ParseCSVFile(filePath)

	if len(invocationTracker) == 0 {
		t.Fatal("No keys defined in resultant invocation tracker.")
	}

	if len(invocationTracker) != 14 {
		t.Fatal("Unexpected number of keys in invocation tracker.")
	}

	/* Test first data row */
	appHash := "aaaaa2c01926d19690e5ec308bab64ef97950b75b1c7582283e0783fce1751d8"
	funcHash := "11111f8758c8c2a20082c161e955405e950439f0503522fe129e709a5dc0e58f"
	uniqueFunctionID := UniqueFunctionID{appHash, funcHash}

	invocationSlice, exists := invocationTracker[uniqueFunctionID]
	if !exists {
		t.Fatal("Example functionID does not exist.")
	}

	if invocationSlice[0].startTime != 1.00000 ||
		invocationSlice[0].duration != 9.000 {
		t.Errorf("Unexpected 'startTime' or 'duration' values. Got %f %f, expected 1.0 and 9.0",
			invocationSlice[0].startTime, invocationSlice[0].duration)
	}

	/* Test function with multiple invocations */
	appHash2 := "bbbbb2c01926d19690e5ec308bab64ef97950b75b1c7582283e0783fce1751d8"
	funcHash2 := "11111f8758c8c2a20082c161e955405e950439f0503522fe129e709a5dc0e58f"
	uniqueFunctionID2 := UniqueFunctionID{appHash2, funcHash2}

	invocationSlice2, exists2 := invocationTracker[uniqueFunctionID2]
	if !exists2 {
		t.Fatal("Multi-invocation function does not exist.")
	}
	if len(invocationSlice2) == 0 {
		t.Fatal("Multi-invocation function is empty.")
	}
	if len(invocationSlice2) != 16 {
		t.Fatal("Multi-invocation function has incorrect number of invocations.")
	}

	sort.Slice(invocationSlice2, func(i, j int) bool {
		return invocationSlice2[i].startTime < invocationSlice2[j].startTime
	})

	expectedTimestamp := [...]float64{122.0, 122.1, 122.2, 122.3, 122.4, 122.5, 122.6, 122.7, 122.8, 122.9, 123.0, 123.1, 123.2, 123.3, 123.4, 123.5}
	expectedDuration := 9.0
	tolerance := 0.00001
	for i, invocation := range invocationSlice2 {
		if math.Abs(invocation.startTime-expectedTimestamp[i]) > tolerance {
			t.Errorf("Incorrect startTime. Expected %f got %f", expectedTimestamp[i], invocation.startTime)
		}
		if math.Abs(invocation.duration-expectedDuration) > tolerance {
			t.Errorf("Incorrect duration. Expected 9.0 got %f", invocation.duration)
		}
	}
}

// Test transformation of InvocationArray into specification struct.
func TestAzure2021GenerateFunctionSpecification(t *testing.T) {
	memoryDefault := 150

	spec1 := common.FunctionSpecification{
		IAT:            common.IATArray{1000000},
		PerMinuteCount: []int{1},
		RuntimeSpecification: common.RuntimeSpecificationArray{
			common.RuntimeSpecification{Runtime: 1000, Memory: memoryDefault},
		},
	}
	spec2 := common.FunctionSpecification{
		IAT:            common.IATArray{1000000, 1000000, 1000000},
		PerMinuteCount: []int{3},
		RuntimeSpecification: common.RuntimeSpecificationArray{
			common.RuntimeSpecification{Runtime: 1000, Memory: memoryDefault},
			common.RuntimeSpecification{Runtime: 1000, Memory: memoryDefault},
			common.RuntimeSpecification{Runtime: 1000, Memory: memoryDefault},
		},
	}
	spec3 := common.FunctionSpecification{
		IAT:                  common.IATArray{},
		PerMinuteCount:       []int{},
		RuntimeSpecification: common.RuntimeSpecificationArray{},
	}
	spec4 := common.FunctionSpecification{
		IAT:            common.IATArray{3000000, 18000000},
		PerMinuteCount: []int{2},
		RuntimeSpecification: common.RuntimeSpecificationArray{
			common.RuntimeSpecification{Runtime: 5000, Memory: memoryDefault},
			common.RuntimeSpecification{Runtime: 6000, Memory: memoryDefault},
		},
	}
	spec5 := common.FunctionSpecification{
		IAT:            common.IATArray{30000000, 41000000, 24000000},
		PerMinuteCount: []int{1, 2},
		RuntimeSpecification: common.RuntimeSpecificationArray{
			common.RuntimeSpecification{Runtime: 20000, Memory: memoryDefault},
			common.RuntimeSpecification{Runtime: 5000, Memory: memoryDefault},
			common.RuntimeSpecification{Runtime: 17000, Memory: memoryDefault},
		},
	}

	tests := map[string]struct {
		// Inputs
		slice           Invocations
		durationMinutes int
		// Outputs
		funcSpec common.FunctionSpecification
		empty    bool
	}{
		"single invocation":       {slice: Invocations{Invocation{1.0, 1.0}}, durationMinutes: 2, funcSpec: spec1, empty: false},
		"triple invocation":       {slice: Invocations{Invocation{1.0, 1.0}, Invocation{2.0, 1.0}, Invocation{3.0, 1.0}}, durationMinutes: 2, funcSpec: spec2, empty: false},
		"outside durationMinutes": {slice: Invocations{Invocation{61.0, 5.0}, Invocation{70.0, 5.0}}, durationMinutes: 1, funcSpec: spec3, empty: true},
		"variable runtime":        {slice: Invocations{Invocation{3.0, 5.0}, Invocation{21.0, 6.0}}, durationMinutes: 2, funcSpec: spec4, empty: false},
		"sparse invocations":      {slice: Invocations{Invocation{30.0, 20.0}, Invocation{71.0, 5.0}, Invocation{95.0, 17.0}}, durationMinutes: 3, funcSpec: spec5, empty: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			funcSpec, empty := GenerateFunctionSpecification(tc.slice, tc.durationMinutes)

			if empty && tc.empty && (funcSpec == nil) { // Invocations outside durationMinutes

			} else if !reflect.DeepEqual(*funcSpec, tc.funcSpec) {
				t.Errorf("expected: %v, got: %v", funcSpec, tc.funcSpec)
			}
		})
	}
}

func ReadOrWriteSpecificationToFile(functions []*common.Function, writeIATsToFile bool, readIATsFromFile bool) {
	if writeIATsToFile && readIATsFromFile {
		log.Fatal("Invalid loader configuration. No point to read and write IATs within the same run.")
	}

	if readIATsFromFile {
		// Parse and read IATs Function Specifications
		for i := range functions {
			var spec common.FunctionSpecification

			iatFile, _ := os.ReadFile("iat" + strconv.Itoa(i) + ".json")
			err := json.Unmarshal(iatFile, &spec)
			if err != nil {
				log.Fatalf("Failed to unmarshal IAT file: %s", err)
			}
			functions[i].Specification = &spec
		}

		log.Info("IATs have been read from file(s).")
	}

	if writeIATsToFile {
		// Writes IATs Function Specifictions to .jsons file
		for i, function := range functions {
			file, _ := json.MarshalIndent(function.Specification, "", " ")
			err := os.WriteFile("iat"+strconv.Itoa(i)+".json", file, 0644)
			if err != nil {
				log.Fatalf("Writing the loader config file failed: %s", err)
			}
		}

		log.Info("IATs have been generated.. The program has exited.")
		os.Exit(0)
	}
}
