package generate

import (
	tc "github.com/eth-easl/loader/pkg/trace"
	"math"
	"sync"
	"testing"
)

type SpecTuple struct {
	runtime int
	memory  int
}

func TestGenerateIAT(t *testing.T) {
	tests := []struct {
		testName        string
		duration        int // s
		invocations     int
		iatDistribution IatDistribution
		expectedPoints  []float64 // μs
	}{
		{
			testName:        "1min_5inv_equidistant",
			duration:        1,
			invocations:     5,
			iatDistribution: Equidistant,
			expectedPoints: []float64{
				200000,
				200000,
				200000,
				200000,
				200000,
			},
		},
	}

	var seed int64 = 123456789
	epsilon := 10e-3

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			InitSeed(seed)

			result := GenerateInterarrivalTimesInMicro(test.duration, test.invocations, test.iatDistribution)
			failed := false

			for i := 0; i < len(result); i++ {
				if math.Abs(result[i]-test.expectedPoints[i]) > epsilon {
					failed = true

					break
				}
			}

			if failed {
				t.Error("Test " + test.testName + " has failed.")
			}
		})
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
