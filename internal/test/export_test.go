package test

import (
	"sync"
	"testing"

	mc "github.com/eth-easl/loader/internal/metric"
	"github.com/stretchr/testify/assert"
)

func TestConcurrentReporting(t *testing.T) {
	exporter := mc.NewExporter()
	var wg sync.WaitGroup

	// This function increments a named counter
	// in a loop.
	doReport := func(t, n int) {
		for i := 0; i < n; i++ {
			if t == 0 {
				exporter.ReportLantency(mc.LatencyRecord{})
			} else {
				exporter.ReportInvocation(mc.MinuteInvocationRecord{})
			}
		}
		wg.Done()
	}

	wg.Add(3)
	go doReport(0, 10_000)
	go doReport(0, 10_000)
	go doReport(1, 10_000)
	wg.Wait()

	assert.Equal(t, 20_000, exporter.GetLantencyRecordLen())
	assert.Equal(t, 10_000, exporter.GetInvocationRecordLen())
}

func TestGetLatenciesInOrder(t *testing.T) {
	exporter := mc.NewExporter()

	exporter.ReportLantency(
		mc.LatencyRecord{
			Timestamp: 1000_000,
			Latency:   0,
		},
	)
	exporter.ReportLantency(
		mc.LatencyRecord{
			Timestamp: 1000_000_000,
			Latency:   2,
		},
	)
	exporter.ReportLantency(
		mc.LatencyRecord{
			Timestamp: 1000,
			Latency:   1,
		},
	)

	assert.EqualValues(t, []float64{1, 0, 2}, exporter.GetLatenciesInOrder())
}
