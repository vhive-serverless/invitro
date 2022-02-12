package trace

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"

	util "github.com/eth-easl/loader/pkg"
	log "github.com/sirupsen/logrus"
)

const (
	gatewayUrl = "192.168.1.240.sslip.io" // Address of the load balancer.
	namespace  = "default"
	port       = "80"
)

func ParseInvocationTrace(traceFile string, traceDuration int) FunctionTraces {
	// Clamp duration to (0, 1440].
	traceDuration = util.MaxOf(util.MinOf(traceDuration, 1440), 1)

	log.Infof("Parsing function invocation trace %s (duration: %dmin)", traceFile, traceDuration)

	var functions []Function
	// Indices of functions to invoke.
	invocationIdices := make([][]int, traceDuration)
	totalInvocations := make([]int, traceDuration)

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	reader := csv.NewReader(csvfile)
	funcIdx := -1
	for {
		// Read each record from csv.
		record, err := reader.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		// Skip header.
		if funcIdx != -1 {
			// Parse invocations.
			var invocations []int
			headerLen := 4
			for i := headerLen; i < headerLen+traceDuration; i++ {
				minute := i - headerLen
				num, err := strconv.Atoi(record[i])
				util.Check(err)

				invocations = append(invocations, num)

				for j := 0; j < num; j++ {
					//* For `num` invocations of function with index `funcIdx`,
					//* we append (N*funcIdx) to the `invocationIdices`.
					invocationIdices[minute] = append(invocationIdices[minute], funcIdx)
				}
				totalInvocations[minute] = totalInvocations[minute] + num
			}

			// Create function profile.
			funcName := fmt.Sprintf("%s-%d", "trace-func", funcIdx)

			function := Function{
				Mame:            funcName,
				Url:             fmt.Sprintf("%s.%s.%s:%s", funcName, namespace, gatewayUrl, port),
				AppHash:         record[1],
				Hash:            record[2],
				InvocationStats: ProfileFunctionInvocations(invocations),
			}
			functions = append(functions, function)
		}
		funcIdx++
	}

	return FunctionTraces{
		Functions:                 functions,
		InvocationsEachMinute:     invocationIdices,
		TotalInvocationsPerMinute: totalInvocations,
		Path:                      traceFile,
	}
}

/** Get execution times in ms. */
func parseDurationStats(record []string) FunctionRuntimeStats {
	return FunctionRuntimeStats{
		Average:       parseToInt(record[3]),
		Count:         parseToInt(record[4]),
		Minimum:       parseToInt(record[5]),
		Maximum:       parseToInt(record[6]),
		Percentile0:   parseToInt(record[7]),
		Percentile1:   parseToInt(record[8]),
		Percentile25:  parseToInt(record[9]),
		Percentile50:  parseToInt(record[10]),
		Percentile75:  parseToInt(record[11]),
		Percentile99:  parseToInt(record[12]),
		Percentile100: parseToInt(record[13]),
	}
}

func parseToInt(text string) int {
	if s, err := strconv.ParseFloat(text, 32); err == nil {
		return int(s)
	} else {
		log.Fatal("Failed to parse duration record", err)
		return 0
	}
}

func ParseDurationTrace(trace *FunctionTraces, traceFile string) {
	log.Infof("Parsing function duration trace: %s", traceFile)

	// Create mapping from function hash to function position in trace
	funcPos := make(map[string]int)
	for i, function := range trace.Functions {
		funcPos[function.Hash] = i
	}

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	reader := csv.NewReader(csvfile)
	l := -1
	foundDurations := 0
	for {
		// Read each record from csv
		record, err := reader.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		// Skip header
		if l != -1 {
			// Parse durations
			functionHash := record[2]
			funcIdx, contained := funcPos[functionHash]
			if contained {
				trace.Functions[funcIdx].RuntimeStats = parseDurationStats(record)
				// //TODO: Move to a better place later.
				// trace.Functions[funcIdx].ConcurrencySats = ProfileFunctionConcurrencies(trace.Functions[funcIdx])
				foundDurations += 1
			}
		}
		l++
	}

	if foundDurations != len(trace.Functions) {
		log.Fatal("Could not find all durations for all invocations in the supplied trace ", foundDurations, len(trace.Functions))
	}
}

/** Get memory usages in MiB. */
func parseMemoryStats(record []string) FunctionMemoryStats {
	return FunctionMemoryStats{
		Count:         parseToInt(record[2]),
		Average:       parseToInt(record[3]),
		Percentile1:   parseToInt(record[4]),
		Percentile5:   parseToInt(record[5]),
		Percentile25:  parseToInt(record[6]),
		Percentile50:  parseToInt(record[7]),
		Percentile75:  parseToInt(record[8]),
		Percentile95:  parseToInt(record[9]),
		Percentile99:  parseToInt(record[10]),
		Percentile100: parseToInt(record[11]),
	}
}

func ParseMemoryTrace(trace *FunctionTraces, traceFile string) {
	log.Infof("Parsing function memory trace: %s", traceFile)

	// Create mapping from function hash to function position in trace
	funcPos := make(map[string]int)
	for i, function := range trace.Functions {
		funcPos[function.AppHash] = i
	}

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	r := csv.NewReader(csvfile)
	l := -1
	foundDurations := 0
	for {
		// Read each record from csv
		record, err := r.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		// Skip header
		if l != -1 {
			// Parse durations
			functionHash := record[1]
			funcIdx, contained := funcPos[functionHash]
			if contained {
				trace.Functions[funcIdx].MemoryStats = parseMemoryStats(record)
				foundDurations += 1
			}
		}
		l++
	}

	if foundDurations != len(trace.Functions) {
		log.Fatal("Could not find all memory footprints for all invocations in the supplied trace ", foundDurations, len(trace.Functions))
	}
}

// // Functions is an object for unmarshalled JSON with functions to deploy.
// type Functions struct {
// 	Functions []FunctionType `json:"functions"`
// }

// type FunctionType struct {
// 	Name string `json:"name"`
// 	File string `json:"file"`

// 	// Number of functions to deploy from the same file (with different names)
// 	Count int `json:"count"`

// 	Eventing    bool   `json:"eventing"`
// 	ApplyScript string `json:"applyScript"`
// }

// func getFuncSlice(file string) []fc.FunctionType {
// 	log.Info("Opening JSON file with functions: ", file)
// 	byteValue, err := ioutil.ReadFile(file)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	var functions fc.Functions
// 	if err := json.Unmarshal(byteValue, &functions); err != nil {
// 		log.Fatal(err)
// 	}
// 	return functions.Functions
// }
