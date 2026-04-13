package metric

import (
	"encoding/csv"
	"math"
	"os"
	"sync"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
)

func RunCSVWriter(records chan interface{}, filename string, writerDone *sync.WaitGroup) {
	log.Debugf("Starting writer for %s", filename)

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()

	writer := gocsv.NewSafeCSVWriter(csv.NewWriter(file))
	if err := gocsv.MarshalChan(records, writer); err != nil {
		log.Fatal(err)
	}

	writerDone.Done()
}

func CreateGlobalMetricsCollector(filename string, collector chan *common.ExecutionRecord,
	signalReady *sync.WaitGroup, signalEverythingWritten *sync.WaitGroup, totalIssuedChannel chan int64) {

	// NOTE: totalNumberOfInvocations is initialized to MaxInt64 not to allow collector to complete before
	// the end signal is received on totalIssuedChannel, which deliver the total number of issued invocations.
	// This number is known once all the individual function drivers finish issuing invocations and
	// when all the invocations return
	var totalNumberOfInvocations int64 = math.MaxInt64
	var currentlyWritten int64

	file, err := os.Create(filename)
	common.Check(err)
	defer file.Close()

	signalReady.Done()

	records := make(chan interface{}, 100)
	writerDone := sync.WaitGroup{}
	writerDone.Add(1)
	go RunCSVWriter(records, filename, &writerDone)

	for {
		select {
		case record := <-collector:
			records <- record

			currentlyWritten++
		case record := <-totalIssuedChannel:
			totalNumberOfInvocations = record
		}

		if currentlyWritten == totalNumberOfInvocations {
			close(records)
			writerDone.Wait()
			(*signalEverythingWritten).Done()

			return
		}
	}
}
