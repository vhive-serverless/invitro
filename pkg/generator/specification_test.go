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

package generator

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

var testFunction = common.Function{
	RuntimeStats: &common.FunctionRuntimeStats{
		Average:       50,
		Count:         100,
		Minimum:       0,
		Maximum:       100,
		Percentile0:   0,
		Percentile1:   1,
		Percentile25:  25,
		Percentile50:  50,
		Percentile75:  75,
		Percentile99:  99,
		Percentile100: 100,
	},
	MemoryStats: &common.FunctionMemoryStats{
		Average:       5000,
		Count:         100,
		Percentile1:   100,
		Percentile5:   500,
		Percentile25:  2500,
		Percentile50:  5000,
		Percentile75:  7500,
		Percentile95:  9500,
		Percentile99:  9900,
		Percentile100: 10000,
	},
}

/*
	TestSerialGenerateIAT tests the following scenarios:

- equidistant distribution within 1 minute and 5 minutes
- uniform distribution - spillover test, distribution test, single point
*/
func TestSerialGenerateIAT(t *testing.T) {
	tests := []struct {
		testName         string
		duration         int // s
		invocations      []int
		iatDistribution  common.IatDistribution
		shiftIAT         bool
		granularity      common.TraceGranularity
		expectedPoints   []float64 // μs
		testDistribution bool
	}{
		{
			testName:         "no_invocations_equidistant",
			invocations:      []int{5},
			iatDistribution:  common.Equidistant,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   []float64{},
			testDistribution: false,
		},
		{
			testName:         "no_invocations_exponential",
			invocations:      []int{5},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   []float64{},
			testDistribution: false,
		},
		{
			testName:         "no_invocations_exponential_shift",
			invocations:      []int{5},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   []float64{},
			testDistribution: false,
		},
		{
			testName:         "one_invocations_exponential",
			invocations:      []int{1},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   []float64{0},
			testDistribution: false,
		},
		{
			testName:         "one_invocations_exponential_shift",
			invocations:      []int{1},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   []float64{0},
			testDistribution: false,
		},
		{
			testName:        "1min_5ipm_equidistant",
			invocations:     []int{5},
			iatDistribution: common.Equidistant,
			shiftIAT:        false,
			granularity:     common.MinuteGranularity,
			expectedPoints: []float64{
				0,
				12000000,
				12000000,
				12000000,
				12000000,
			},
			testDistribution: false,
		},
		{
			testName:        "5min_5ipm_equidistant",
			invocations:     []int{5, 5, 5, 5, 5},
			iatDistribution: common.Equidistant,
			shiftIAT:        false,
			granularity:     common.MinuteGranularity,
			expectedPoints: []float64{
				// min 1
				0,
				12000000,
				12000000,
				12000000,
				12000000,
				// min 2
				12000000,
				12000000,
				12000000,
				12000000,
				12000000,
				// min 3
				12000000,
				12000000,
				12000000,
				12000000,
				12000000,
				// min 4
				12000000,
				12000000,
				12000000,
				12000000,
				12000000,
				// min 5
				12000000,
				12000000,
				12000000,
				12000000,
				12000000,
			},
			testDistribution: false,
		},
		{
			testName:         "1min_1000000ipm_uniform",
			invocations:      []int{1000000},
			iatDistribution:  common.Uniform,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   nil,
			testDistribution: true,
		},
		{
			testName:         "1min_1000000ipm_exponential",
			invocations:      []int{1000000},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   nil,
			testDistribution: true,
		},
		{
			testName:        "2sec_5qps_equidistant",
			invocations:     []int{5, 4, 2},
			iatDistribution: common.Equidistant,
			shiftIAT:        false,
			granularity:     common.SecondGranularity,
			expectedPoints: []float64{
				// second 1 - μs below
				0,
				200000,
				200000,
				200000,
				200000,
				// second 2 - μs below
				250000,
				250000,
				250000,
				250000,
				// second 3 - μs below
				500000,
				500000,
			},
			testDistribution: false,
		},
	}

	var seed int64 = 123456789
	epsilon := 10e-3

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			sg := NewSpecificationGenerator(seed)

			testFunction.InvocationStats = &common.FunctionInvocationStats{Invocations: test.invocations}
			spec := sg.GenerateInvocationData(&testFunction, test.iatDistribution, test.shiftIAT, test.granularity)
			IAT, perMinuteCount, nonScaledDuration := spec.IAT, spec.PerMinuteCount, spec.RawDuration

			failed := false

			/*if hasSpillover(IAT, perMinuteCount, test.granularity) {
				t.Error("Generated IAT does not fit in the within the minute time window.")
			}*/

			if test.expectedPoints != nil {
				for i := 0; i < len(test.expectedPoints); i++ {
					if len(test.expectedPoints) != len(IAT) {
						log.Debug(fmt.Sprintf("wrong number of IATs in the minute, got: %d, expected: %d\n", len(IAT), len(test.expectedPoints)))

						failed = true
						break
					}

					if math.Abs(IAT[i]-test.expectedPoints[i]) > epsilon {
						log.Debug(fmt.Sprintf("got: %f, expected: %f\n", IAT[i], test.expectedPoints[i]))

						failed = true
						// no break statement for debugging purpose
					}
				}

				if failed {
					t.Error("Test " + test.testName + " has failed due to incorrectly generated IAT.")
				}
			}

			if test.testDistribution && test.iatDistribution != common.Equidistant &&
				!checkDistribution(IAT, perMinuteCount, nonScaledDuration, test.iatDistribution) {

				t.Error("The provided sample does not satisfy the given distribution.")
			}
		})
	}
}

/*func hasSpillover(data []float64, perMinuteCount []int, granularity common.TraceGranularity) bool {
	beginIndex := 0
	endIndex := perMinuteCount[0]

	for min := 0; min < len(perMinuteCount); min++ {
		sum := 0.0
		epsilon := 1e-3

		for i := beginIndex; i < endIndex; i++ {
			sum += data[i]
		}

		if min+1 < len(perMinuteCount) {
			beginIndex += perMinuteCount[min]
			endIndex = beginIndex + perMinuteCount[min+1]
		}

		log.Debug(fmt.Sprintf("Total execution time: %f μs\n", sum))

		spilloverThreshold := common.OneSecondInMicroseconds
		if granularity == common.MinuteGranularity {
			spilloverThreshold *= 60
		}

		if math.Abs(sum-spilloverThreshold) > epsilon {
			return true
		}
	}

	return false
}*/

func checkDistribution(data []float64, perMinuteCount []int, nonScaledDuration []float64, distribution common.IatDistribution) bool {
	// PREPARING ARGUMENTS
	var dist string
	inputFile := "test_data.txt"

	switch distribution {
	case common.Uniform:
		dist = "uniform"
	case common.Exponential:
		dist = "exponential"
	default:
		log.Fatal("Unsupported distribution check")
	}

	result := false

	beginIndex := 0
	endIndex := perMinuteCount[0]

	for min := 0; min < len(perMinuteCount); min++ {
		// WRITING DISTRIBUTION TO TEST
		f, err := os.Create(inputFile)
		if err != nil {
			log.Fatal("Cannot write data for distribution tests.")
		}

		defer f.Close()

		for i := beginIndex; i < endIndex; i++ {
			_, _ = f.WriteString(fmt.Sprintf("%f\n", data[i]))
		}

		if min+1 < len(perMinuteCount) {
			beginIndex += perMinuteCount[min]
			endIndex = beginIndex + perMinuteCount[min+1]
		}

		// SETTING UP THE TESTING SCRIPT
		args := []string{"specification_statistical_test.py", dist, inputFile, fmt.Sprintf("%f", nonScaledDuration[min])}
		statisticalTest := exec.Command("python3", args...)

		// CALLING THE TESTING SCRIPT AND PROCESSING ITS RESULTS
		// NOTE: the script generates a histogram in PNG format that can be used as a sanity-check
		if err := statisticalTest.Wait(); err != nil {
			output, _ := statisticalTest.Output()
			log.Debug(string(output))

			switch statisticalTest.ProcessState.ExitCode() {
			case 0:
				result = true // distribution satisfied
			case 1:
				return false // distribution not satisfied
			case 2:
				log.Fatal("Unsupported distribution by the statistical test.")
			}
		}
	}

	return result
}

func TestGenerateExecutionSpecifications(t *testing.T) {
	tests := []struct {
		testName    string
		iterations  int
		granularity common.TraceGranularity
		expected    map[common.RuntimeSpecification]struct{}
	}{
		{
			testName:    "exec_spec_run_1",
			iterations:  1,
			granularity: common.MinuteGranularity,
			expected: map[common.RuntimeSpecification]struct{}{
				common.RuntimeSpecification{Runtime: 89, Memory: 8217}: {},
			},
		},
		{
			testName:    "exec_spec_run_5",
			iterations:  5,
			granularity: common.MinuteGranularity,
			expected: map[common.RuntimeSpecification]struct{}{
				common.RuntimeSpecification{Runtime: 89, Memory: 8217}: {},
				common.RuntimeSpecification{Runtime: 18, Memory: 9940}: {},
				common.RuntimeSpecification{Runtime: 50, Memory: 1222}: {},
				common.RuntimeSpecification{Runtime: 85, Memory: 7836}: {},
				common.RuntimeSpecification{Runtime: 67, Memory: 7490}: {},
			},
		},
		{
			testName:    "exec_spec_run_25",
			iterations:  25,
			granularity: common.MinuteGranularity,
			expected: map[common.RuntimeSpecification]struct{}{
				common.RuntimeSpecification{Runtime: 89, Memory: 8217}:  {},
				common.RuntimeSpecification{Runtime: 18, Memory: 9940}:  {},
				common.RuntimeSpecification{Runtime: 67, Memory: 7490}:  {},
				common.RuntimeSpecification{Runtime: 50, Memory: 1222}:  {},
				common.RuntimeSpecification{Runtime: 90, Memory: 193}:   {},
				common.RuntimeSpecification{Runtime: 85, Memory: 7836}:  {},
				common.RuntimeSpecification{Runtime: 24, Memory: 4875}:  {},
				common.RuntimeSpecification{Runtime: 42, Memory: 5785}:  {},
				common.RuntimeSpecification{Runtime: 82, Memory: 6819}:  {},
				common.RuntimeSpecification{Runtime: 22, Memory: 9838}:  {},
				common.RuntimeSpecification{Runtime: 11, Memory: 2223}:  {},
				common.RuntimeSpecification{Runtime: 81, Memory: 2832}:  {},
				common.RuntimeSpecification{Runtime: 99, Memory: 5305}:  {},
				common.RuntimeSpecification{Runtime: 99, Memory: 6582}:  {},
				common.RuntimeSpecification{Runtime: 58, Memory: 4581}:  {},
				common.RuntimeSpecification{Runtime: 25, Memory: 1813}:  {},
				common.RuntimeSpecification{Runtime: 79, Memory: 9819}:  {},
				common.RuntimeSpecification{Runtime: 2, Memory: 1660}:   {},
				common.RuntimeSpecification{Runtime: 98, Memory: 3110}:  {},
				common.RuntimeSpecification{Runtime: 18, Memory: 6178}:  {},
				common.RuntimeSpecification{Runtime: 3, Memory: 7770}:   {},
				common.RuntimeSpecification{Runtime: 100, Memory: 4063}: {},
				common.RuntimeSpecification{Runtime: 6, Memory: 5022}:   {},
				common.RuntimeSpecification{Runtime: 35, Memory: 8003}:  {},
				common.RuntimeSpecification{Runtime: 20, Memory: 3544}:  {},
			},
		},
	}

	var seed int64 = 123456789

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			sg := NewSpecificationGenerator(seed)

			results := make(map[common.RuntimeSpecification]struct{})

			wg := sync.WaitGroup{}
			mutex := sync.Mutex{}

			testFunction.InvocationStats = &common.FunctionInvocationStats{
				Invocations: []int{test.iterations},
			}
			// distribution is irrelevant here
			spec := sg.GenerateInvocationData(&testFunction, common.Equidistant, false, test.granularity).RuntimeSpecification

			for i := 0; i < test.iterations; i++ {
				wg.Add(1)

				index := i
				go func() {
					runtime, memory := spec[index].Runtime, spec[index].Memory

					mutex.Lock()
					results[common.RuntimeSpecification{Runtime: runtime, Memory: memory}] = struct{}{}
					mutex.Unlock()

					wg.Done()
				}()
			}

			wg.Wait()

			for got := range results {
				if _, ok := results[got]; !ok {
					t.Error("Missing value for runtime specification.")
				}
			}
		})
	}
}
