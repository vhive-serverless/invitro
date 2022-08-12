package trace

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	util "github.com/eth-easl/loader/pkg"
	log "github.com/sirupsen/logrus"
)

const (
	bareMetalLbGateway = "10.200.3.4.sslip.io" // Address of the bare-metal load balancer.
	namespace          = "default"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

var GetFuncEndpoint = func(name string) string {
	return fmt.Sprintf("%s.%s.%s", name, namespace, bareMetalLbGateway)
}

func ParseInvocationTrace(traceFile string, traceDuration int) FunctionTraces {
	// Clamp duration to (0, 1440].
	traceDuration = util.MaxOf(util.MinOf(traceDuration, 1440), 1)

	log.Infof("Parsing function invocation trace %s (duration: %dmin)", traceFile, traceDuration)

	var functions []Function
	// Indices of functions to invoke.
	invocationIndices := make([][]int, traceDuration)
	totalInvocations := make([]int, traceDuration)

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	reader := csv.NewReader(csvfile)
	funcIdx := -1

	hashOwnerIndex := -1
	hashAppIndex := -1
	hashFunctionIndex := -1
	invocationColumnIndex := -1

	for {
		// Read each record from csv.
		record, err := reader.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if funcIdx == -1 {
			// Parse header.
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
				log.Fatal("Invocation trace does not contain one of the hashes.")
			}

			if invocationColumnIndex == -1 {
				invocationColumnIndex = 3
			}
		} else {
			// Parse invocations.
			var invocations []int

			for i := invocationColumnIndex; i < invocationColumnIndex+traceDuration; i++ {
				minute := i - invocationColumnIndex
				num, err := strconv.Atoi(record[i])
				util.Check(err)

				invocations = append(invocations, num)

				for j := 0; j < num; j++ {
					//* For `num` invocations of function with index `funcIdx`,
					//* we append (N*funcIdx) to the `invocationIndices`.
					invocationIndices[minute] = append(invocationIndices[minute], funcIdx)
				}
				totalInvocations[minute] = totalInvocations[minute] + num
			}

			// Create function profile.
			funcName := fmt.Sprintf("%s-%d-%d", "trace-func", funcIdx, rand.Uint64())

			function := Function{
				Name:                    funcName,
				Endpoint:                GetFuncEndpoint(funcName),
				HashOwner:               record[hashOwnerIndex],
				HashApp:                 record[hashAppIndex],
				HashFunction:            record[hashFunctionIndex],
				NumInvocationsPerMinute: invocations,

				InvocationStats: ProfileFunctionInvocations(invocations),
			}
			functions = append(functions, function)
		}
		funcIdx++
	}

	return FunctionTraces{
		Functions:                 functions,
		InvocationsEachMinute:     invocationIndices,
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

	// Create mapping from function hash to function position in `FunctionTraces`.
	funcPos := make(map[string]int)
	for i, function := range trace.Functions {
		if _, exist := funcPos[function.HashFunction]; exist {
			//* Replace duplicated function hash.
			function.HashFunction = strconv.Itoa(int(util.Hash(function.Name)))
		}
		funcPos[function.HashFunction] = i
	}

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	reader := csv.NewReader(csvfile)
	l := -1
	foundDurations := 0

	functionHashIndex := -1

	for {
		// Read each record from csv
		record, err := reader.Read()

		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if l == -1 {
			// Parse header
			for i := 0; i < 3; i++ {
				if strings.ToLower(record[i]) == "hashfunction" {
					functionHashIndex = i
					break
				}
			}

			if functionHashIndex == -1 {
				log.Fatal("Invalid duration trace. No function hash.")
			}
		} else {
			functionHash := record[functionHashIndex]
			funcIdx, contained := funcPos[functionHash]
			if contained {
				trace.Functions[funcIdx].RuntimeStats = parseDurationStats(record)
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
	memMap := make(map[string]FunctionMemoryStats)
	appFuncCntMap := make(map[string]int)

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	// Count number of functions for each app
	for _, function := range trace.Functions {
		appFuncCntMap[function.HashApp] += 1
	}

	r := csv.NewReader(csvfile)
	l := -1
	hashAppIndex := -1

	// Parse memory stats for each app
	for {
		record, err := r.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Fatal(err)
		}

		if l == -1 {
			for i := 0; i < 2; i++ {
				if strings.ToLower(record[i]) == "hashapp" {
					hashAppIndex = i
					break
				}
			}

			if hashAppIndex == -1 {
				log.Fatal("Memory trace is missing hash app column.")
			}
		} else {
			hashApp := record[hashAppIndex]
			fncCnt := appFuncCntMap[hashApp]
			if fncCnt < 1 {
				log.Fatal("Function is missing for app " + hashApp)
			}

			if fncCnt == 1 {
				memMap[hashApp] = parseMemoryStats(record)
				continue
			}

			appMem := parseMemoryStats(record)
			funcAvgMem := FunctionMemoryStats{
				Average:       appMem.Average / fncCnt,
				Count:         appMem.Count / fncCnt,
				Percentile1:   appMem.Percentile1 / fncCnt,
				Percentile5:   appMem.Percentile5 / fncCnt,
				Percentile25:  appMem.Percentile25 / fncCnt,
				Percentile50:  appMem.Percentile50 / fncCnt,
				Percentile75:  appMem.Percentile75 / fncCnt,
				Percentile95:  appMem.Percentile95 / fncCnt,
				Percentile99:  appMem.Percentile99 / fncCnt,
				Percentile100: appMem.Percentile100 / fncCnt,
			}
			memMap[hashApp] = funcAvgMem

		}
		l++
	}
	var foundMemories int

	// Assign apps memory to each function
	for idx := range trace.Functions {
		if mem, ok := memMap[trace.Functions[idx].HashApp]; ok {
			trace.Functions[idx].MemoryStats = mem
			foundMemories++
		}
	}

	if foundMemories != len(trace.Functions) {
		log.Fatal("Could not find all memory footprints for all invocations in the supplied trace ", foundMemories, len(trace.Functions))
	}
}
