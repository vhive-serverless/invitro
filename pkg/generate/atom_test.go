package generate

import (
	"fmt"
	tc "github.com/eth-easl/loader/pkg/trace"
	"math"
	"strconv"
	"sync"
	"testing"
)

type SpecTuple struct {
	runtime int
	memory  int
}

func TestSerialGenerateIAT(t *testing.T) {
	tests := []struct {
		testName        string
		duration        int // s
		invocations     []int
		iatDistribution IatDistribution
		expectedPoints  []float64 // μs
	}{
		{
			testName:        "1min_5ipm_equidistant",
			invocations:     []int{5},
			iatDistribution: Equidistant,
			expectedPoints: []float64{
				200000,
				200000,
				200000,
				200000,
				200000,
			},
		},
		{
			testName:        "5min_5ipm_equidistant",
			invocations:     []int{5, 5, 5, 5, 5},
			iatDistribution: Equidistant,
			expectedPoints: []float64{
				// min 1
				200000,
				200000,
				200000,
				200000,
				200000,
				// min 2
				200000,
				200000,
				200000,
				200000,
				200000,
				// min 3
				200000,
				200000,
				200000,
				200000,
				200000,
				// min 4
				200000,
				200000,
				200000,
				200000,
				200000,
				// min 5
				200000,
				200000,
				200000,
				200000,
				200000,
			},
		},
		{
			testName:        "1min_25ipm_uniform",
			invocations:     []int{25},
			iatDistribution: Uniform,
			expectedPoints: []float64{
				30599.122571,
				41608.158237,
				38196.324414,
				30578.882235,
				22764.889978,
				76016.766045,
				16592.728276,
				35329.100169,
				52099.267210,
				40731.321212,
				32513.437415,
				23390.447643,
				70713.474644,
				43441.555898,
				33898.690187,
				43378.264279,
				37630.168228,
				16738.151876,
				55379.881960,
				43053.128652,
				46935.569044,
				9619.049539,
				73383.110897,
				42199.634584,
				39102.216455,
			},
		},
		{
			testName:        "1min_6000ipm_uniform",
			invocations:     []int{6000},
			iatDistribution: Uniform,
			expectedPoints:  nil,
		},
	}

	var seed int64 = 123456789
	epsilon := 10e-3

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			InitSeed(seed)

			result := GenerateIAT(test.invocations, test.iatDistribution)
			failed := false

			if testForSpillover(len(test.invocations), result) {
				t.Error("Generated IAT does not fit in the " + strconv.Itoa(len(test.invocations)) + " minutes time window.")
			}

			if test.expectedPoints != nil {
				for i := 0; i < len(result); i++ {
					if math.Abs(result[i]-test.expectedPoints[i]) > epsilon {
						fmt.Printf("got: %f, expected: %f\n", result[i], test.expectedPoints[i])

						failed = true
						//break
					}
				}

				if failed {
					t.Error("Test " + test.testName + " has failed due to incorrectly generated IAT.")
				}
			}

			// TODO: it would make sense to check for uniform distribution and/or Poisson using statistical tests

		})
	}
}

func testForSpillover(maxTime int, data []float64) bool {
	sum := 0.0

	for i := 0; i < len(data); i++ {
		sum += data[i]
	}

	if sum > float64(maxTime)*oneSecondInMicro {
		return true
	} else {
		return false
	}
}

func TestGenerateExecutionSpecifications(t *testing.T) {
	fakeFunction := tc.Function{
		RuntimeStats: tc.FunctionRuntimeStats{
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
		MemoryStats: tc.FunctionMemoryStats{
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

	tests := []struct {
		testName   string
		iterations int
		expected   map[SpecTuple]struct{}
	}{
		{
			testName:   "exec_spec_run_1",
			iterations: 1,
			expected: map[SpecTuple]struct{}{
				SpecTuple{runtime: 89, memory: 8217}: {},
			},
		},
		{
			testName:   "exec_spec_run_5",
			iterations: 5,
			expected: map[SpecTuple]struct{}{
				SpecTuple{runtime: 89, memory: 8217}: {},
				SpecTuple{runtime: 18, memory: 9940}: {},
				SpecTuple{runtime: 50, memory: 1222}: {},
				SpecTuple{runtime: 85, memory: 7836}: {},
				SpecTuple{runtime: 67, memory: 7490}: {},
			},
		},
		{
			testName:   "exec_spec_run_25",
			iterations: 25,
			expected: map[SpecTuple]struct{}{
				SpecTuple{runtime: 89, memory: 8217}:  {},
				SpecTuple{runtime: 18, memory: 9940}:  {},
				SpecTuple{runtime: 67, memory: 7490}:  {},
				SpecTuple{runtime: 50, memory: 1222}:  {},
				SpecTuple{runtime: 90, memory: 193}:   {},
				SpecTuple{runtime: 85, memory: 7836}:  {},
				SpecTuple{runtime: 24, memory: 4875}:  {},
				SpecTuple{runtime: 42, memory: 5785}:  {},
				SpecTuple{runtime: 82, memory: 6819}:  {},
				SpecTuple{runtime: 22, memory: 9838}:  {},
				SpecTuple{runtime: 11, memory: 2223}:  {},
				SpecTuple{runtime: 81, memory: 2832}:  {},
				SpecTuple{runtime: 99, memory: 5305}:  {},
				SpecTuple{runtime: 99, memory: 6582}:  {},
				SpecTuple{runtime: 58, memory: 4581}:  {},
				SpecTuple{runtime: 25, memory: 1813}:  {},
				SpecTuple{runtime: 79, memory: 9819}:  {},
				SpecTuple{runtime: 2, memory: 1660}:   {},
				SpecTuple{runtime: 98, memory: 3110}:  {},
				SpecTuple{runtime: 18, memory: 6178}:  {},
				SpecTuple{runtime: 3, memory: 7770}:   {},
				SpecTuple{runtime: 100, memory: 4063}: {},
				SpecTuple{runtime: 6, memory: 5022}:   {},
				SpecTuple{runtime: 35, memory: 8003}:  {},
				SpecTuple{runtime: 20, memory: 3544}:  {},
			},
		},
	}

	var seed int64 = 123456789

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			InitSeed(seed)

			results := make(map[SpecTuple]struct{})

			wg := sync.WaitGroup{}
			mutex := sync.Mutex{}

			for i := 0; i < test.iterations; i++ {
				wg.Add(1)

				go func() {
					runtime, memory := GenerateExecutionSpecs(fakeFunction)

					mutex.Lock()
					results[SpecTuple{runtime: runtime, memory: memory}] = struct{}{}
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
