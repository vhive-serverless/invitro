package generate

import (
	"fmt"
	tc "github.com/eth-easl/loader/pkg/trace"
	"math"
	"os"
	"os/exec"
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
				12000000,
				12000000,
				12000000,
				12000000,
				12000000,
			},
		},
		{
			testName:        "5min_5ipm_equidistant",
			invocations:     []int{5, 5, 5, 5, 5},
			iatDistribution: Equidistant,
			expectedPoints: []float64{
				// min 1
				12000000,
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
			testName:        "1min_1000000ipm_uniform",
			invocations:     []int{1000000},
			iatDistribution: Uniform,
			expectedPoints:  nil,
		},
		{
			testName:        "1min_1000000ipm_exponential",
			invocations:     []int{1000000},
			iatDistribution: Exponential,
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
						break
					}
				}

				if failed {
					t.Error("Test " + test.testName + " has failed due to incorrectly generated IAT.")
				}
			} else {
				satisfiesDistribution := checkDistribution(result, 0, 1, test.iatDistribution)

				if !satisfiesDistribution {
					t.Error("The provided sample does not satisfy the given distribution.")
				}
			}
		})
	}
}

func testForSpillover(maxTime int, data []float64) bool {
	sum := 0.0

	for i := 0; i < len(data); i++ {
		sum += data[i]
	}

	if sum > float64(maxTime)*60*oneSecondInMicro {
		return true
	} else {
		return false
	}
}

func checkDistribution(data []float64, min, max float64, distribution IatDistribution) bool {
	// PREPARING ARGUMENTS
	var dist string
	switch distribution {
	case Uniform:
		dist = "uniform"
	case Exponential:
		dist = "exponential"
	default:
		panic("Unsupported distribution check")
	}

	minBoundary := fmt.Sprintf("%f", min)
	maxBoundary := fmt.Sprintf("%f", max)
	inputFile := "test_data.txt"

	// WRITING DISTRIBUTION TO TEST
	f, err := os.Create(inputFile)
	if err != nil {
		panic("err")
	}

	defer f.Close()

	for _, iat := range data {
		f.WriteString(fmt.Sprintf("%f\n", iat))
	}

	// CALLING THE TESTING SCRIPT
	args := []string{"specification_statistical_test.py", dist, minBoundary, maxBoundary, inputFile}
	statisticalTest := exec.Command("python3.8", args...)
	if err := statisticalTest.Start(); err != nil {
		panic("Failed to the data against the given distribution.")
	}

	// CHECKING FOR RESULT
	if err := statisticalTest.Wait(); err != nil {
		switch statisticalTest.ProcessState.ExitCode() {
		case 200:
			return true // distribution satisfied
		case 300:
			panic("Unsupported distribution by the statistical test.")
		case 400:
			return false // distribution not satisfied
		}
	}

	return false
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
