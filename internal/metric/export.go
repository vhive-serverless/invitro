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

	util "github.com/eth-easl/loader/internal"
	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
)

type Exporter struct {
	mutex             sync.Mutex
	invocationRecords []MinuteInvocationRecord
	latencyRecords    []LatencyRecord
}

type AdfResult struct {
	TestStats    float64 `json:"statistics"`
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

func NewExporter() Exporter {
	return Exporter{
		//* Note that the zero value of a mutex is usable as-is, so no
		//* initialization is required here. (mutex: sync.Mutex{})
		invocationRecords: []MinuteInvocationRecord{},
		latencyRecords:    []LatencyRecord{},
	}
}

func (ep *Exporter) HasReachedStationarity(pvalue float64) bool {
	latencies := ep.GetLatenciesInOrder()
	latenciesStr := strings.Trim(strings.Join(
		strings.Fields(fmt.Sprint(latencies)), " "), "[]")

	cmd := exec.Command(
		"python3",
		"internal/metric/run_adf.py",
		latenciesStr,
	)
	out, err := cmd.CombinedOutput()
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

	log.Info(result)

	return pvalue >= result.Pvalue
}

func (ep *Exporter) GetLatenciesInOrder() []float64 {
	ep.sortLatencyRecords()

	lantencies := make([]float64, len(ep.latencyRecords))
	for i, record := range ep.latencyRecords {
		lantencies[i] = float64(record.Latency)
	}
	return lantencies
}

// Sort records in ascending order.
func (ep *Exporter) sortLatencyRecords() {
	sort.Slice(ep.latencyRecords,
		func(i, j int) bool {
			return ep.latencyRecords[i].Timestamp < ep.latencyRecords[j].Timestamp
		},
	)
}

func (ep *Exporter) FinishAndSave(phase int, duration int) {
	invocFileName := "data/out/invoke_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	invocF, err := os.Create(invocFileName)
	util.Check(err)
	gocsv.MarshalFile(&ep.invocationRecords, invocF)

	latencyFileName := "data/out/latency_phase-" + strconv.Itoa(phase) + "_dur-" + strconv.Itoa(duration) + ".csv"
	latencyF, err := os.Create(latencyFileName)
	util.Check(err)
	gocsv.MarshalFile(&ep.latencyRecords, latencyF)
}

func (ep *Exporter) ReportInvocation(record MinuteInvocationRecord) {
	ep.mutex.Lock()
	defer ep.mutex.Unlock()
	ep.invocationRecords = append(ep.invocationRecords, record)
}

func (ep *Exporter) ReportLantency(record LatencyRecord) {
	ep.mutex.Lock()
	defer ep.mutex.Unlock()
	ep.latencyRecords = append(ep.latencyRecords, record)
}

func (ep *Exporter) GetInvocationRecordLen() int {
	return len(ep.invocationRecords)
}

func (ep *Exporter) GetLantencyRecordLen() int {
	return len(ep.latencyRecords)
}
