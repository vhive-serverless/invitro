package metric

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"

	util "github.com/eth-easl/loader/pkg"
	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
)

type Exporter struct {
	mutex             sync.Mutex
	invocationRecords []MinuteInvocationRecord
	executionRecords  []ExecutionRecord
}

type ClusterUsage struct {
	Cpu          []string `json:"cpu"`
	CpuPctAvg    float64  `json:"cpu_pct"`
	Memory       []string `json:"memory"`
	MemoryPctAvg float64  `json:"memory_pct"`
}

type AdfResult struct {
	TestStats    float64 `json:"statistic"`
	Pvalue       float64 `json:"pvalue"`
	Lag          int     `json:"usedlag"`
	NumObs       int     `json:"nobs"`
	CriticalVals TValues `json:"critical_vals"`
	IcBest       float64 `json:"icbest"`
}

type TValues struct {
	Pct1  float64 `json:"1%"`
	Pct5  float64 `json:"5%"`
	Pct10 float64 `json:"10%"`
}

func ScrapeClusterUsage() ClusterUsage {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_infra.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal("Fail to scrape cluster usage: ", err)
	}

	var result ClusterUsage
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Fatal("Fail to parse cluster usage: ", string(out[:]), err)
	}

	return result
}

const SLOWDOWN_THRESHOLD = 10

func (ep *Exporter) CheckOverload(window int, failureRatio float64) bool {
	checkSlowdown := func(responseTime int64, runtime uint32) bool {
		return responseTime/int64(runtime) >= SLOWDOWN_THRESHOLD
	}

	if window < 0 {
		window = len(ep.executionRecords)
	}
	failureCount := 0
	for _, record := range ep.executionRecords[len(ep.executionRecords)-window:] {
		if record.Timeout || record.Failed ||
			checkSlowdown(record.ResponseTime, record.Runtime) {
			failureCount += 1
		}
	}
	// log.Info(failureCount)
	return float64(failureCount)/float64(len(ep.executionRecords)) >= failureRatio
}

const LATENCY_WINDOW = 500

func (ep *Exporter) IsLatencyStationary(pvalue float64) bool {
	latencies := ep.GetLatenciesInOrder()
	if len(latencies) <= 3 {
		return true
	}

	//* Window the measurements to prevent overflow.
	if len(latencies) > LATENCY_WINDOW {
		latencies = latencies[len(latencies)-LATENCY_WINDOW:]
	}
	//* Here `-` is used to form a single cmd argument to prevent the violation of the calling convention.
	latenciesStr := strings.Trim(strings.Join(
		strings.Fields(fmt.Sprint(latencies)), "-"), "[]")

	cmd := exec.Command(
		"python3",
		"pkg/metric/run_adf.py",
		// "run_adf.py", //* For testing (due to POSX constraint)
		latenciesStr,
	)
	out, err := cmd.CombinedOutput()
	// log.Info(string(out[:]))
	if err != nil {
		log.Warn("Fail to run ADF test: ", err)
		return false
	}

	var result AdfResult
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse ADF test result: ", string(out[:]), err)
		return false
	}
	// log.Info(result)

	isStationary := pvalue >= result.Pvalue
	switch {
	case pvalue <= 0.01:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct1)
	case pvalue <= 0.05:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct5)
	case pvalue <= 0.10:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct10)
	}
	return isStationary
}

func (ep *Exporter) GetLatenciesInOrder() []float64 {
	ep.sortExecutionRecordsByTime()

	lantencies := make([]float64, len(ep.executionRecords))
	for i, record := range ep.executionRecords {
		lantencies[i] = float64(record.ResponseTime) - float64(record.Runtime)
	}
	return lantencies
}

// Sort records in ascending order.
func (ep *Exporter) sortExecutionRecordsByTime() {
	sort.Slice(ep.executionRecords,
		func(i, j int) bool {
			return ep.executionRecords[i].Timestamp < ep.executionRecords[j].Timestamp
		},
	)
}

func NewExporter() Exporter {
	return Exporter{
		//* Note that the zero value of a mutex is usable as-is, so no
		//* initialization is required here (e.g., mutex: sync.Mutex{}).
		invocationRecords: []MinuteInvocationRecord{},
		executionRecords:  []ExecutionRecord{},
	}
}

func (ep *Exporter) FinishAndSave(sampleSize int, phase int, duration int) {
	if sampleSize > 0 {
		invocFileName := "data/out/inv_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
		invocF, err := os.Create(invocFileName)
		util.Check(err)
		gocsv.MarshalFile(&ep.invocationRecords, invocF)
	}

	latencyFileName := "data/out/exec_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	util.Check(err)
	gocsv.MarshalFile(&ep.executionRecords, latencyF)
}

func (ep *Exporter) ReportInvocation(record MinuteInvocationRecord) {
	ep.mutex.Lock()
	defer ep.mutex.Unlock()
	ep.invocationRecords = append(ep.invocationRecords, record)
}

func (ep *Exporter) ReportExecution(record ExecutionRecord) {
	ep.mutex.Lock()
	defer ep.mutex.Unlock()
	ep.executionRecords = append(ep.executionRecords, record)
}

func (ep *Exporter) GetInvocationRecordLen() int {
	return len(ep.invocationRecords)
}

func (ep *Exporter) GetLantencyRecordLen() int {
	return len(ep.executionRecords)
}
