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
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNexusTraceParserWithRealData(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "nexus-parser-test-real")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a dummy invocations.csv file with content from data/traces/nexus/invocations.csv
	invocationCsvContent := `FunctionName,1,2,3,4,5
pyaesserve-s3-rpc,5,5,5,5,5
pyaesserve-s3-rpc,1,1,1,1,1
pyaesserve-s3-rpc,2,2,2,2,2
`
	invocationsPath := filepath.Join(tempDir, "invocations.csv")
	err = os.WriteFile(invocationsPath, []byte(invocationCsvContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write dummy csv file: %v", err)
	}

	durationCsvContent := `FunctionName,AvgDurationMs
pyaesserve-s3-rpc,23.477
pyaesserve-s3-rpc,23.477
pyaesserve-s3-rpc,23.477
`
	durationPath := filepath.Join(tempDir, "durations.csv")
	err = os.WriteFile(durationPath, []byte(durationCsvContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write dummy csv file: %v", err)
	}
	// Create a parser instance
	parser := NewNexusParser(tempDir, 200, "test.yaml")

	// Parse the trace
	functions := parser.Parse()
	for _, function := range functions {
		fmt.Printf("Parsed function: %+v\n", function)
	}

	// Assertions
	if len(functions) != 3 {
		t.Fatalf("Expected 3 functions, but got %d", len(functions))
	}
}
