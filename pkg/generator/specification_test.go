package generator

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"sync"
	"testing"

	"github.com/eth-easl/loader/pkg/common"
	log "github.com/sirupsen/logrus"
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
		expectedPoints   [][]float64 // μs
		testDistribution bool
	}{
		{
			testName:         "no_invocations_equidistant",
			invocations:      []int{5},
			iatDistribution:  common.Equidistant,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   [][]float64{},
			testDistribution: false,
		},
		{
			testName:         "no_invocations_exponential",
			invocations:      []int{5},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   [][]float64{},
			testDistribution: false,
		},
		{
			testName:         "no_invocations_exponential_shift",
			invocations:      []int{5},
			iatDistribution:  common.Exponential,
			shiftIAT:         true,
			granularity:      common.MinuteGranularity,
			expectedPoints:   [][]float64{},
			testDistribution: false,
		},
		{
			testName:         "one_invocations_exponential",
			invocations:      []int{1},
			iatDistribution:  common.Exponential,
			shiftIAT:         false,
			granularity:      common.MinuteGranularity,
			expectedPoints:   [][]float64{{0, 60000000}},
			testDistribution: false,
		},
		{
			testName:         "one_invocations_exponential_shift",
			invocations:      []int{1},
			iatDistribution:  common.Exponential,
			shiftIAT:         true,
			granularity:      common.MinuteGranularity,
			expectedPoints:   [][]float64{{11689078.788397, 48310921.211603}},
			testDistribution: false,
		},
		{
			testName:        "1min_5ipm_equidistant",
			invocations:     []int{5},
			iatDistribution: common.Equidistant,
			shiftIAT:        false,
			granularity:     common.MinuteGranularity,
			expectedPoints: [][]float64{
				{
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
			},
			testDistribution: false,
		},
		{
			testName:        "5min_5ipm_equidistant",
			invocations:     []int{5, 5, 5, 5, 5},
			iatDistribution: common.Equidistant,
			shiftIAT:        false,
			granularity:     common.MinuteGranularity,
			expectedPoints: [][]float64{
				{
					// min 1
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
				{
					// min 2
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
				{
					// min 3
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
				{
					// min 4
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
				{
					// min 5
					0,
					12000000,
					12000000,
					12000000,
					12000000,
					12000000,
				},
			},
			testDistribution: false,
		},
		{
			testName:        "1min_25ipm_uniform_shift",
			invocations:     []int{25},
			iatDistribution: common.Uniform,
			shiftIAT:        true,
			granularity:     common.MinuteGranularity,
			expectedPoints: [][]float64{
				{
					1193000.964808,
					622524.819620,
					2161625.000293,
					2467158.610498,
					3161216.965226,
					120925.338482,
					3461650.068734,
					3681772.563419,
					3591929.298027,
					3062124.611863,
					3223056.707367,
					3042558.740794,
					2099765.805752,
					375008.683565,
					3979289.345154,
					1636869.797787,
					1169442.102841,
					2380243.616007,
					2453428.612640,
					1704231.066313,
					42074.939233,
					3115643.026141,
					3460047.444726,
					2849475.331077,
					3187546.011741,
					1757390.527891,
				},
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
			testName:        "1min_25ipm_exponential",
			invocations:     []int{25},
			iatDistribution: common.Exponential,
			shiftIAT:        false,
			granularity:     common.MinuteGranularity,
			expectedPoints: [][]float64{
				{
					0,
					1311929.341329,
					3685871.430916,
					1626476.996595,
					556382.014270,
					30703.105102,
					3988584.779392,
					2092271.836277,
					1489855.293253,
					3025094.199801,
					2366337.4678820,
					40667.5994150,
					2778945.4898700,
					4201722.5747150,
					5339421.1460450,
					3362048.1584080,
					939526.5236740,
					1113771.3822940,
					4439636.5676460,
					4623026.1098310,
					2082985.6557600,
					45937.1189860,
					4542253.8756200,
					2264414.9939920,
					3872560.8680640,
					179575.4708620,
				},
			},
			testDistribution: false,
		},
		{
			testName:        "1min_25ipm_exponential_shift",
			invocations:     []int{25},
			iatDistribution: common.Exponential,
			shiftIAT:        true,
			granularity:     common.MinuteGranularity,
			expectedPoints: [][]float64{
				{
					697544.471476,
					5339421.146045,
					3362048.158408,
					939526.523674,
					1113771.382294,
					4439636.567646,
					4623026.109831,
					2082985.655760,
					45937.118986,
					4542253.875620,
					2264414.993992,
					3872560.868064,
					179575.470862,
					1311929.341329,
					3685871.430916,
					1626476.996595,
					556382.014270,
					30703.105102,
					3988584.779392,
					2092271.836277,
					1489855.293253,
					3025094.199801,
					2366337.4678820,
					40667.5994150,
					2778945.4898700,
					3504178.103239,
				},
			},
			testDistribution: false,
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
			expectedPoints: [][]float64{
				{
					// second 1
					0,
					200000,
					200000,
					200000,
					200000,
					200000,
				},
				{
					// second 2
					0,
					250000,
					250000,
					250000,
					250000,
				},
				{
					// second 3
					0,
					500000,
					500000,
				},
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
			IAT, nonScaledDuration := spec.IAT, spec.RawDuration

			failed := false

			if hasSpillover(IAT, test.granularity) {
				t.Error("Generated IAT does not fit in the within the minute time window.")
			}

			if test.expectedPoints != nil {
				for min := 0; min < len(test.expectedPoints); min++ {
					for i := 0; i < len(test.expectedPoints[min]); i++ {
						if len(test.expectedPoints[min]) != len(IAT[min]) {
							log.Debug(fmt.Sprintf("wrong number of IATs in the minute, got: %d, expected: %d\n", len(IAT[min]), len(test.expectedPoints[min])))

							failed = true
							break
						}
						if math.Abs(IAT[min][i]-test.expectedPoints[min][i]) > epsilon {
							log.Debug(fmt.Sprintf("got: %f, expected: %f\n", IAT[min][i], test.expectedPoints[min][i]))

							failed = true
							// no break statement for debugging purpose
						}
					}
				}

				if failed {
					t.Error("Test " + test.testName + " has failed due to incorrectly generated IAT.")
				}
			}

			if test.testDistribution && test.iatDistribution != common.Equidistant &&
				!checkDistribution(IAT, nonScaledDuration, test.iatDistribution) {

				t.Error("The provided sample does not satisfy the given distribution.")
			}
		})
	}
}

func hasSpillover(data [][]float64, granularity common.TraceGranularity) bool {
	for min := 0; min < len(data); min++ {
		sum := 0.0
		epsilon := 1e-3

		for i := 0; i < len(data[min]); i++ {
			sum += data[min][i]
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
}

func checkDistribution(data [][]float64, nonScaledDuration []float64, distribution common.IatDistribution) bool {
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

	for min := 0; min < len(data); min++ {
		// WRITING DISTRIBUTION TO TEST
		f, err := os.Create(inputFile)
		if err != nil {
			log.Fatal("Cannot write data for distribution tests.")
		}

		defer f.Close()

		for _, iat := range data[min] {
			_, _ = f.WriteString(fmt.Sprintf("%f\n", iat))
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
					runtime, memory := spec[0][index].Runtime, spec[0][index].Memory

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
