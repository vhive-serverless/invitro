package trace

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"strconv"

	log "github.com/sirupsen/logrus"
)

const (
	gatewayUrl = "192.168.1.240.sslip.io"
	namespace  = "default"
	port       = "80"
)

type FunctionDurationStats struct {
	average       int
	count         int
	minimum       int
	maximum       int
	percentile0   int
	percentile1   int
	percentile25  int
	percentile50  int
	percentile75  int
	percentile99  int
	percentile100 int
}

type FunctionMemoryStats struct {
	average       int
	percentile1   int
	percentile5   int
	percentile25  int
	percentile50  int
	percentile75  int
	percentile95  int
	percentile99  int
	percentile100 int
}
type Function struct {
	name          string
	url           string
	appHash       string
	hash          string
	deployed      bool
	durationStats FunctionDurationStats
	memoryStats   FunctionMemoryStats
}

type FunctionTrace struct {
	path                    string
	Functions               []Function
	InvocationsPerMin       [][]int
	TotalInvocationsEachMin []int
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()
}

func (f *Function) SetHash(hash int) {
	f.hash = fmt.Sprintf("%015d", hash)
}

func (f *Function) SetName(name string) {
	f.name = name
}

func (f *Function) SetStatus(b bool) {
	f.deployed = b
}

func (f *Function) GetStatus() bool {
	return f.deployed
}

func (f *Function) GetName() string {
	return f.name
}

func (f *Function) GetUrl() string {
	return f.url
}

func (f *Function) SetUrl(url string) {
	f.url = url
}

func ShuffleInvocations(trace FunctionTrace, traceDuration int) {
	for t := 0; t < traceDuration; t++ {
		rand.Shuffle(len(trace.InvocationsPerMin[t]), func(i, j int) {
			trace.InvocationsPerMin[t][i], trace.InvocationsPerMin[t][j] = trace.InvocationsPerMin[t][j], trace.InvocationsPerMin[t][i]
		})
	}
}

func ParseInvocationTrace(traceFile string, traceDuration int) FunctionTrace {
	log.Infof("Parsing function invocation trace: %s", traceFile)

	var functions []Function
	// Array functions (position) to invoke
	invocations := make([][]int, traceDuration)
	totalInvocations := make([]int, traceDuration)

	csvfile, err := os.Open(traceFile)
	if err != nil {
		log.Fatal("Failed to load CSV file", err)
	}

	r := csv.NewReader(csvfile)
	l := -1
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
			// Parse function
			function := Function{appHash: record[1], hash: record[2]}
			function.name = fmt.Sprintf("%s-%d", "trace-func", l)
			function.url = fmt.Sprintf("%s.%s.%s:%s", function.name, namespace, gatewayUrl, port)
			functions = append(functions, function)

			// Parse invocations
			for i := 4; i < 4+traceDuration; i++ {
				num, err := strconv.Atoi(record[i])
				if err != nil {
					log.Fatal("Failed to parse number", err)
				}
				for j := 0; j < num; j++ {
					invocations[i-4] = append(invocations[i-4], l)
				}
				totalInvocations[i-4] = totalInvocations[i-4] + num
			}
		}
		l++
	}

	return FunctionTrace{
		Functions:               functions,
		InvocationsPerMin:       invocations,
		TotalInvocationsEachMin: totalInvocations,
		path:                    traceFile,
	}
}

func getDurationStats(record []string) FunctionDurationStats {
	return FunctionDurationStats{
		average:       parseToInt(record[3]),
		count:         parseToInt(record[4]),
		minimum:       parseToInt(record[5]),
		maximum:       parseToInt(record[6]),
		percentile0:   parseToInt(record[7]),
		percentile1:   parseToInt(record[8]),
		percentile25:  parseToInt(record[9]),
		percentile50:  parseToInt(record[10]),
		percentile75:  parseToInt(record[11]),
		percentile99:  parseToInt(record[12]),
		percentile100: parseToInt(record[13]),
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

func ParseDurationTrace(trace *FunctionTrace, traceFile string) {
	log.Infof("Parsing function duration trace: %s", traceFile)

	// Create mapping from function hash to function position in trace
	funcPos := make(map[string]int)
	for i, function := range trace.Functions {
		funcPos[function.hash] = i
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
			functionHash := record[2]
			funcIdx, contained := funcPos[functionHash]
			if contained {
				trace.Functions[funcIdx].durationStats = getDurationStats(record)
				foundDurations += 1
			}
		}
		l++
	}

	if foundDurations != len(trace.Functions) {
		log.Fatal("Could not find all durations for all invocations in the supplied trace ", foundDurations, len(trace.Functions))
	}
}

func getMemoryStats(record []string) FunctionMemoryStats {
	return FunctionMemoryStats{
		average:       parseToInt(record[3]),
		percentile1:   parseToInt(record[4]),
		percentile5:   parseToInt(record[5]),
		percentile25:  parseToInt(record[6]),
		percentile50:  parseToInt(record[7]),
		percentile75:  parseToInt(record[8]),
		percentile95:  parseToInt(record[9]),
		percentile99:  parseToInt(record[10]),
		percentile100: parseToInt(record[11]),
	}
}

func ParseMemoryTrace(trace *FunctionTrace, traceFile string) {
	log.Infof("Parsing function memory trace: %s", traceFile)

	// Create mapping from function hash to function position in trace
	funcPos := make(map[string]int)
	for i, function := range trace.Functions {
		funcPos[function.appHash] = i
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
				trace.Functions[funcIdx].memoryStats = getMemoryStats(record)
				foundDurations += 1
			}
		}
		l++
	}

	if foundDurations != len(trace.Functions) {
		log.Fatal("Could not find all memory footprints for all invocations in the supplied trace ", foundDurations, len(trace.Functions))
	}
}

// Make better later
// Now also picking the percentile value, that's generally too high
func GetExecutionSpecification(function Function) (int, int) {
	// get function runtime and memory usage
	percentile := rand.Intn(100) + 1 // rand gives numbers [0,100) but we need [1,100]
	var runtime, memory int
	runtimePercentile := function.durationStats
	memoryPercentile := function.memoryStats
	switch {
	case percentile > 99:
		runtime = runtimePercentile.percentile100
		memory = memoryPercentile.percentile100
	case percentile > 95:
		runtime = runtimePercentile.percentile99
		memory = memoryPercentile.percentile99
	case percentile > 75:
		runtime = runtimePercentile.percentile99
		memory = memoryPercentile.percentile95
	case percentile > 50:
		runtime = runtimePercentile.percentile75
		memory = memoryPercentile.percentile75
	case percentile > 25:
		runtime = runtimePercentile.percentile50
		memory = memoryPercentile.percentile50
	case percentile > 5:
		runtime = runtimePercentile.percentile25
		memory = memoryPercentile.percentile25
	case percentile > 1:
		runtime = runtimePercentile.percentile25
		memory = memoryPercentile.percentile5
	default:
		runtime = runtimePercentile.percentile1
		memory = memoryPercentile.percentile1
	}
	return runtime, memory
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
