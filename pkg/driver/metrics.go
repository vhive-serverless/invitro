package driver

import (
	"encoding/json"
	"github.com/vhive-serverless/loader/pkg/common"
	mc "github.com/vhive-serverless/loader/pkg/metric"
	"os"
	"sync"
	"time"
)

func (d *Driver) CreateMetricsScrapper(interval time.Duration,
	signalReady *sync.WaitGroup, finishCh chan int, allRecordsWritten *sync.WaitGroup) func() {
	timer := time.NewTicker(interval)

	return func() {
		signalReady.Done()
		knStatRecords := make(chan interface{}, 100)
		scaleRecords := make(chan interface{}, 100)
		writerDone := sync.WaitGroup{}

		clusterUsageFile, err := os.Create(d.outputFilename("cluster_usage"))
		common.Check(err)
		defer clusterUsageFile.Close()

		writerDone.Add(1)
		go mc.RunCSVWriter(knStatRecords, d.outputFilename("kn_stats"), &writerDone)

		writerDone.Add(1)
		go mc.RunCSVWriter(scaleRecords, d.outputFilename("deployment_scale"), &writerDone)

		for {
			select {
			case <-timer.C:
				recCluster := mc.ScrapeClusterUsage()
				recCluster.Timestamp = time.Now().UnixMicro()

				byteArr, err := json.Marshal(recCluster)
				common.Check(err)

				_, err = clusterUsageFile.Write(byteArr)
				common.Check(err)

				_, err = clusterUsageFile.WriteString("\n")
				common.Check(err)

				recScale := mc.ScrapeDeploymentScales()
				timestamp := time.Now().UnixMicro()
				for _, rec := range recScale {
					rec.Timestamp = timestamp
					scaleRecords <- rec
				}

				recKnative := mc.ScrapeKnStats()
				recKnative.Timestamp = time.Now().UnixMicro()
				knStatRecords <- recKnative
			case <-finishCh:
				close(knStatRecords)
				close(scaleRecords)

				writerDone.Wait()
				allRecordsWritten.Done()

				return
			}
		}
	}
}
