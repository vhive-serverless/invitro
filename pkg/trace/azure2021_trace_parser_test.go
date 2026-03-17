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
	"testing"

	// Temp
	"encoding/json"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

// TODO, separate into 2 functions for better testing.
func TestFunction(t *testing.T) {
	tracePath := "test_data/AzureFunctionsInvocationTraceForTwoWeeksJan2021.csv"
	durationToParse := 10
	yamlPath := "dummy"

	traceParser := NewAzure2021Parser(tracePath, durationToParse, yamlPath)
	functions := traceParser.Parse()

	ReadOrWriteSpecificationToFile(functions, true, false)

	durationToParse = 21
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
