package metric

import (
	"encoding/json"
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	util "github.com/eth-easl/loader/pkg"
	tc "github.com/eth-easl/loader/pkg/trace"
	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
)

type Collector struct {
	mutex             sync.Mutex
	invocationRecords []MinuteInvocationRecord
	executionRecords  []ExecutionRecord
	scaleRecords      []ScaleRecord
	scaleRegistry     ScaleRegistry
}

func NewCollector() Collector {
	registry := ScaleRegistry{}
	registry.Init(ScrapeDeploymentScales())
	return Collector{
		//* Note that the zero value of a mutex is usable as-is, so no initialization is required here (e.g., mutex: sync.Mutex{}).
		invocationRecords: []MinuteInvocationRecord{},
		executionRecords:  []ExecutionRecord{},
		scaleRecords:      []ScaleRecord{},
		scaleRegistry:     registry,
	}
}

func (collector *Collector) GetOneColdStartFunction() common.Function {
	funcName := collector.scaleRegistry.GetOneColdFunctionName()
	return common.Function{
		Name:     funcName,
		Endpoint: tc.GetFuncEndpoint(funcName),
	}
}

func (collector *Collector) RecordScalesAndGetColdStartCount() int {
	scales := ScrapeDeploymentScales()
	collector.scaleRecords = append(collector.scaleRecords, scales...)
	return collector.scaleRegistry.UpdateAndGetColdStartCount(scales)
}

func ScrapeDeploymentScales() []ScaleRecord {
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

	timestamp := time.Now().UnixMicro()
	records := []ScaleRecord{}
	for _, result := range results {
		records = append(records, ScaleRecord{
			Timestamp:    timestamp,
			Deployment:   result.Deployment,
			DesiredScale: result.DesiredScale,
			ActualScale:  result.ActualScale,
		})
	}
	return records
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
	cmd := exec.Command("python3", "pkg/metric/scrape_infra.py")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Warn("Fail to scrape cluster usage: ", err)
	}

	var result ClusterUsage
	err = json.Unmarshal(out, &result)
	if err != nil {
		log.Warn("Fail to parse cluster usage: ", string(out[:]), err)
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
		strings.Fields(fmt.Sprint(latencies)), "@"), "[]")

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
		lantencies[i] = float64(record.ResponseTime) - float64(record.ActualDuration)
	}
	return lantencies
}

// Sort records in ascending order.
func (collector *Collector) sortExecutionRecordsByTime() {
	sort.Slice(collector.executionRecords,
		func(i, j int) bool {
			return collector.executionRecords[i].StartTime < collector.executionRecords[j].StartTime
		},
	)
}

func (collector *Collector) FinishAndSave(sampleSize int, phase int, duration int) {
	invocFileName := "data/out/inv_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	invocF, err := os.Create(invocFileName)
	util.Check(err)
	err = gocsv.MarshalFile(&collector.invocationRecords, invocF)
	util.Check(err)
	defer invocF.Close()

	latencyFileName := "data/out/exec_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	util.Check(err)
	err = gocsv.MarshalFile(&collector.executionRecords, latencyF)
	util.Check(err)
	defer latencyF.Close()

	scaleFileName := "data/out/scale_sample-" + strconv.Itoa(sampleSize) + "_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	scaleF, err := os.Create(scaleFileName)
	util.Check(err)
	err = gocsv.MarshalFile(&collector.scaleRecords, scaleF)
	util.Check(err)
	defer scaleF.Close()
}

func (collector *Collector) ReportInvocation(record MinuteInvocationRecord) {
	collector.mutex.Lock()
	defer collector.mutex.Unlock()
	collector.invocationRecords = append(collector.invocationRecords, record)
}

var prevSchedulingP50 = 0.0
var prevSchedulingP99 = 0.0
var prevE2ePlacementP50 = 0.0
var prevE2ePlacementP99 = 0.0

func (collector *Collector) ReportExecution(record ExecutionRecord, clusterUsage ClusterUsage, knStats KnStats) {
	collector.mutex.Lock()
	defer collector.mutex.Unlock()

	record.MasterCpu = clusterUsage.MasterCpuPct
	record.MasterMem = clusterUsage.MasterMemoryPct
	record.WorkerCpuAvg = clusterUsage.CpuPctAvg
	record.WorkerCpuActiveAvg = clusterUsage.CpuPctActiveAvg
	record.WorkerCpuMax = clusterUsage.CpuPctMax
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

	if record.SchedulingP50 = knStats.SchedulingP50; record.SchedulingP50 < 0 {
		record.SchedulingP50 = prevSchedulingP50
	} else {
		prevE2ePlacementP50 = record.SchedulingP50
	}
	if record.SchedulingP99 = knStats.SchedulingP99; record.SchedulingP99 < 0 {
		record.SchedulingP99 = prevSchedulingP99
	} else {
		prevE2ePlacementP99 = record.SchedulingP99
	}
	if record.E2ePlacementP50 = knStats.E2ePlacementP50; record.E2ePlacementP50 < 0 {
		record.E2ePlacementP50 = prevE2ePlacementP50
	} else {
		prevE2ePlacementP50 = record.E2ePlacementP50
	}
	if record.E2ePlacementP99 = knStats.E2ePlacementP99; record.E2ePlacementP99 < 0 {
		record.E2ePlacementP99 = prevE2ePlacementP99
	} else {
		prevE2ePlacementP99 = record.E2ePlacementP99
	}

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
		if record.ConnectionTimeout || record.FunctionTimeout {
			failureCounter += 1
		}
	}
	//* Failure rate w.r.t. the window period.
	failureRate := float64(failureCounter) / float64(windowSize)
	log.Info("Failure count=", failureCounter, " Failure rate=", failureRate)
	return failureRate >= OVERFLOAD_THRESHOLD
}
