package metric

import (
	"os"
	"sort"
	"strconv"
	"sync"

	adfTest "github.com/berkmancenter/adf"
	util "github.com/eth-easl/loader/internal"
	"github.com/gocarina/gocsv"
)

type Exporter struct {
	mutex             sync.Mutex
	invocationRecords []MinuteInvocationRecord
	latencyRecords    []LatencyRecord
}

func NewExporter() Exporter {
	return Exporter{
		//* Note that the zero value of a mutex is usable as-is, so no
		//* initialization is required here. (mutex: sync.Mutex{})
		invocationRecords: []MinuteInvocationRecord{},
		latencyRecords:    []LatencyRecord{},
	}
}

func (ep *Exporter) IsLatencyStationary() bool {
	latencies := ep.GetLatenciesInOrder()
	sort.Float64s(latencies)

	//* Use default p-value threshold (-3.45) and default lag value.
	//TODO: Write a python script instead.
	adf := adfTest.New(ep.GetLatenciesInOrder(), 0, -1)
	adf.Run()
	return adf.IsStationary()
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
