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
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
)

type Collector struct {
	mutex             sync.Mutex
	invocationRecords []MinuteInvocationRecord
	executionRecords  []ExecutionRecord
	scaleRegistry     ScaleRegistry
}

func (collector *Collector) GetColdStartCount() int {
	scales := ScrapeDeploymentScales()
	return collector.scaleRegistry.UpdateAndGetColdStartCount(scales)
}

func ScrapeDeploymentScales() []DeploymentScale {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_scales.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape deployment scales: ", err)
	}

	var results []DeploymentScale
	err = json.Unmarshal(out, &results)
	if err != nil {
		log.Warn("Fail to parse deployment scales: ", string(out[:]), err)
	}

	return results
}

func ScrapeKnStats() KnStats {
	cmd := exec.Command(
		"python3",
		"pkg/metric/scrape_kn.py",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape Knative: ", err)
	}

	var result KnStats
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse Knative: ", string(out[:]), err)
	}

	return result
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

func (collector *Collector) IsLatencyStationary(windowSize int, pvalueThreshold float64) bool {
	latencies := collector.GetLatenciesInOrder()
	if len(latencies) <= 3 {
		return true
	}

	//* Window the measurements to prevent overflow.
	if len(latencies) > windowSize {
		latencies = latencies[len(latencies)-windowSize:]
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

	isStationary := result.Pvalue <= pvalueThreshold
	switch {
	case pvalueThreshold <= 0.01:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct1)
	case pvalueThreshold <= 0.05:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct5)
	case pvalueThreshold <= 0.10:
		isStationary = isStationary &&
			(result.TestStats <= result.CriticalVals.Pct10)
	}

	log.Info("Stationary test: ", isStationary, " P-value=", result.Pvalue)
	return isStationary
}

func (collector *Collector) GetLatenciesInOrder() []float64 {
	collector.sortExecutionRecordsByTime()

	lantencies := make([]float64, len(collector.executionRecords))
	for i, record := range collector.executionRecords {
		lantencies[i] = float64(record.ResponseTime) - float64(record.Runtime)
	}
	return lantencies
}

// Sort records in ascending order.
func (collector *Collector) sortExecutionRecordsByTime() {
	sort.Slice(collector.executionRecords,
		func(i, j int) bool {
			return collector.executionRecords[i].Timestamp < collector.executionRecords[j].Timestamp
		},
	)
}

func NewCollector(functions []tc.Function) Collector {
	registry := ScaleRegistry{}
	registry.Init(functions)
	return Collector{
		//* Note that the zero value of a mutex is usable as-is, so no
		//* initialization is required here (e.g., mutex: sync.Mutex{}).
		invocationRecords: []MinuteInvocationRecord{},
		executionRecords:  []ExecutionRecord{},
		scaleRegistry:     registry,
	}
}

func (collector *Collector) FinishAndSave(sampleSize int, phase int, duration int) {
	if sampleSize > 0 {
		invocFileName := "data/out/inv_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
		invocF, err := os.Create(invocFileName)
		util.Check(err)
		gocsv.MarshalFile(&collector.invocationRecords, invocF)
	}

	latencyFileName := "data/out/exec_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	util.Check(err)
	gocsv.MarshalFile(&collector.executionRecords, latencyF)
}

func (collector *Collector) ReportInvocation(record MinuteInvocationRecord) {
	collector.mutex.Lock()
	defer collector.mutex.Unlock()
	collector.invocationRecords = append(collector.invocationRecords, record)
}

func (collector *Collector) ReportExecution(record ExecutionRecord, clusterUsage ClusterUsage, knStats KnStats) {
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	record.MasterCpu = clusterUsage.MasterCpuPct
	record.MasterMem = clusterUsage.MasterMemoryPct
	record.WorkerCpuAvg = clusterUsage.CpuPctAvg
	record.WorkerMemAvg = clusterUsage.MemoryPctAvg

	record.DesiredPods = knStats.DesiredPods
	record.UnreadyPods = knStats.UnreadyPods
	record.PendingPods = knStats.PendingPods
	record.RequestedPods = knStats.RequestedPods
	record.RunningPods = knStats.RunningPods

	record.ActivatorQueue = knStats.ActivatorQueue
	record.ActivatorRequestCount = knStats.ActivatorRequestCount
	record.AutoscalerStableQueue = knStats.AutoscalerStableQueue
	record.AutoscalerPanicQueue = knStats.AutoscalerPanicQueue

	collector.executionRecords = append(collector.executionRecords, record)
}

func (collector *Collector) GetInvocationRecordLen() int {
	return len(collector.invocationRecords)
}

func (collector *Collector) GetLantencyRecordLen() int {
	return len(collector.executionRecords)
}

const OVERFLOAD_THRESHOLD = 0.2

func (collector *Collector) CheckOverloadDeprecated(windowSize int) bool {
	//* Skip the first time slot that is potentially unstable.
	duration := len(collector.executionRecords)
	if duration <= 2*windowSize {
		return false
	}

	failureCounter := 0
	for _, record := range collector.executionRecords[duration-windowSize:] {
		if record.Timeout || record.Failed {
			failureCounter += 1
		}
	}
	//* Failure rate w.r.t. the window period.
	failureRate := float64(failureCounter) / float64(windowSize)
	log.Info("Failure count=", failureCounter, " Failure rate=", failureRate)
	return failureRate >= OVERFLOAD_THRESHOLD
}
