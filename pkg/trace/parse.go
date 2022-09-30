package trace

import (
	"encoding/csv"
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

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

func ParseInvocationTrace(traceFile string, traceDuration int) common.FunctionTraces {
	// Clamp duration to (0, 1440].
	traceDuration = common.MaxOf(common.MinOf(traceDuration, 1440), 1)

	log.Infof("Parsing function invocation trace %s (duration: %d min)", traceFile, traceDuration)

	var functions []common.Function
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
				common.Check(err)

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

			function := common.Function{
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

	return common.FunctionTraces{
		Path:                      traceFile,
		Functions:                 functions,
		InvocationsEachMinute:     invocationIndices,
		TotalInvocationsPerMinute: totalInvocations,
	}
}

/** Get execution times in ms. */
func parseDurationStats(record []string) common.FunctionRuntimeStats {
	return common.FunctionRuntimeStats{
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

func ParseDurationTrace(trace *common.FunctionTraces, traceFile string) {
	log.Infof("Parsing function duration trace: %s", traceFile)

	// Create a mapping from function hash to function position in `FunctionTraces`.
	funcPosMap := make(map[string]int)
	// Ceate a map from duplicated function hash to new hashes.
	duplicatedFunctionMap := make(map[string][]string)

	for i, function := range trace.Functions {
		if _, existed := funcPosMap[function.HashFunction]; existed {
			newHash := strconv.Itoa(int(common.Hash(function.Name)))
			duplicates, added := duplicatedFunctionMap[function.HashFunction]
			if added {
				duplicatedFunctionMap[function.HashFunction] = append(duplicates, newHash)
			} else {
				duplicatedFunctionMap[function.HashFunction] =
					[]string{function.HashFunction, newHash} //* Add both the original hash and the new hash
			}
			//* Replace duplicated function hash.
			function.HashFunction = newHash
		}
		funcPosMap[function.HashFunction] = i
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
				panic("Invalid duration trace. No function hash.")
			}
		} else {
			functionHash := record[functionHashIndex]
			duplicates, duplicated := duplicatedFunctionMap[functionHash]
			if duplicated {
				//* If it's duplicated, then we choose a hash from one of the new function hashes.
				functionHash = duplicates[len(duplicates)-1]
				//* Delete the used function hash.
				duplicatedFunctionMap[functionHash] = duplicates[:len(duplicates)-1]
			}

			funcIdx, contained := funcPosMap[functionHash]
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
func parseMemoryStats(record []string, fncCnt int) common.FunctionMemoryStats {
	return common.FunctionMemoryStats{
		Count:         parseToInt(record[2]) / fncCnt,
		Average:       parseToInt(record[3]) / fncCnt,
		Percentile1:   parseToInt(record[4]) / fncCnt,
		Percentile5:   parseToInt(record[5]) / fncCnt,
		Percentile25:  parseToInt(record[6]) / fncCnt,
		Percentile50:  parseToInt(record[7]) / fncCnt,
		Percentile75:  parseToInt(record[8]) / fncCnt,
		Percentile95:  parseToInt(record[9]) / fncCnt,
		Percentile99:  parseToInt(record[10]) / fncCnt,
		Percentile100: parseToInt(record[11]) / fncCnt,
	}
}

func ParseMemoryTrace(trace *common.FunctionTraces, traceFile string) {
	log.Infof("Parsing function memory trace: %s", traceFile)

	// Create a mapping from app hash to function position in `FunctionTraces`.
	funcPosMap := make(map[string]int)
	// Ceate a map from duplicated app hash to new hashes.
	duplicatedAppMap := make(map[string][]string)
	// Create a mapping from app hash to the # functions that belong to this app.
	functionCounts := make(map[string]int)

	for i, function := range trace.Functions {
		_, existed := funcPosMap[function.HashApp]
		if existed {
			newHash := strconv.Itoa(int(common.Hash(function.Name)))
			duplicates, added := duplicatedAppMap[function.HashApp]
			if added {
				duplicatedAppMap[function.HashApp] = append(duplicates, newHash)
			} else {
				duplicatedAppMap[function.HashFunction] =
					[]string{function.HashApp, newHash} //* Add both the original hash and the new hash
			}
			//* Replace duplicated function hash.
			function.HashApp = newHash
		}
		if existed {
			//* If the app contains multiple functions, update the function count.
			//* (The update may happen several times until the end.)
			functionCounts[function.HashApp] = len(duplicatedAppMap[function.HashApp])
		} else {
			functionCounts[function.HashApp] = 1
		}
		funcPosMap[function.HashApp] = i
	}

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	r := csv.NewReader(csvfile)
	l := -1
	foundDurations := 0

	hashAppIndex := -1

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
			// Parse durations
			hashApp := record[hashAppIndex]
			duplicates, duplicated := duplicatedAppMap[hashApp]
			if duplicated {
				//* If it's duplicated, then we choose a hash from one of the new function hashes.
				hashApp = duplicates[len(duplicates)-1]
				//* Delete the used app hash.
				duplicatedAppMap[hashApp] = duplicates[:len(duplicates)-1]
			}

			funcIdx, contained := funcPosMap[hashApp]
			if contained {
				trace.Functions[funcIdx].MemoryStats = parseMemoryStats(record, functionCounts[hashApp])
				foundDurations += 1
			}
		}
		l++
	}

	if foundDurations != len(trace.Functions) {
		log.Fatal("Could not find all memory footprints for all invocations in the supplied trace ", foundDurations, len(trace.Functions))
	}
}
