package test

import (
	"sync"
	"testing"

	mc "github.com/eth-easl/loader/pkg/metric"
	"github.com/stretchr/testify/assert"
)

var clusterUsage = mc.ClusterUsage{}
var knStats = mc.KnStats{}

func TestCheckOverload(t *testing.T) {
	collector := mc.NewCollector()
	collector.ReportExecution(
		mc.ExecutionRecord{
			FunctionTimeout: true,
		},
		clusterUsage,
		knStats,
	)
	collector.ReportExecution(
		mc.ExecutionRecord{},
		clusterUsage,
		knStats,
	)
	assert.False(t, collector.CheckOverloadDeprecated(2))

	collector.ReportExecution(
		mc.ExecutionRecord{
			FunctionTimeout: true,
		},
		clusterUsage,
		knStats,
	)
	collector.ReportExecution(
		mc.ExecutionRecord{},
		clusterUsage,
		knStats,
	)
	collector.ReportExecution(
		mc.ExecutionRecord{
			ConnectionTimeout: true,
		},
		clusterUsage,
		knStats,
	)
	assert.False(t, collector.CheckOverloadDeprecated(3))
	assert.True(t, collector.CheckOverloadDeprecated(2))
}

func TestConcurrentReporting(t *testing.T) {
	collector := mc.NewCollector()
	var wg sync.WaitGroup

	// This function increments a named counter
	// in a loop.
	doReport := func(t, n int) {
		for i := 0; i < n; i++ {
			if t == 0 {
				collector.ReportExecution(mc.ExecutionRecord{}, clusterUsage, knStats)
			} else {
				collector.ReportInvocation(mc.MinuteInvocationRecord{})
			}
		}
		wg.Done()
	}

	wg.Add(3)
	go doReport(0, 10_000)
	go doReport(1, 10_000)
	go doReport(0, 10_000)
	wg.Wait()

	assert.Equal(t, 20_000, collector.GetLantencyRecordLen())
	assert.Equal(t, 10_000, collector.GetInvocationRecordLen())
}

func TestGetLatenciesInOrder(t *testing.T) {
	collector := mc.NewCollector()

	collector.ReportExecution(
		mc.ExecutionRecord{
			StartTime:    1000_000,
			ResponseTime: 0,
		},
		clusterUsage,
		knStats,
	)
	collector.ReportExecution(
		mc.ExecutionRecord{
			StartTime:    1000_000_000,
			ResponseTime: 2,
		},
		clusterUsage,
		knStats,
	)
	collector.ReportExecution(
		mc.ExecutionRecord{
			StartTime:    1000,
			ResponseTime: 1,
		},
		clusterUsage,
		knStats,
	)

	assert.EqualValues(t, []float64{1, 0, 2}, collector.GetLatenciesInOrder())
}

// func TestIsLatencyStationary(t *testing.T) {
// 	collector := mc.NewExporter()

// 	for i := 0; i < 100; i++ {
// 		collector.ReportLantency(
// 			mc.LatencyRecord{
// 				StartTime: int64(i),
// 				Latency:   int64(i + 5000),
// 			},
// 		)
// 	}
// 	assert.False(t, collector.IsLatencyStationary(0.05))
// 	assert.True(t, collector.IsLatencyStationary(0.98))
// }
