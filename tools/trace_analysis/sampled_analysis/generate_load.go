package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
)

func convertMemoryToCPU(memory int) float64 {
	// Numbers taken from: https://cloud.google.com/functions/pricing
	if memory <= 128 {
		return 0.083
	} else if memory <= 256 {
		return 0.167
	} else if memory <= 512 {
		return 0.333
	} else if memory <= 1024 {
		return 0.583
	} else if memory <= 2048 {
		return 1
	} else {
		return 2
	}
}

func main() {
	var wg sync.WaitGroup
	var wg2 sync.WaitGroup
	gen.InitSeed(42)
	writer := make(chan string)
	sampleSize := 1500
	var traces tc.FunctionTraces

	invPath := "data/traces/" + strconv.Itoa(sampleSize) + "_inv.csv"
	runPath := "data/traces/" + strconv.Itoa(sampleSize) + "_run.csv"
	memPath := "data/traces/" + strconv.Itoa(sampleSize) + "_mem.csv"

	traces = tc.ParseInvocationTrace(invPath, 1440)
	tc.ParseDurationTrace(&traces, runPath)
	tc.ParseMemoryTrace(&traces, memPath)

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")

	invocationsEachMinute := traces.InvocationsEachMinute[:]
	gen.ShuffleAllInvocationsInplace(&invocationsEachMinute)

	totalDurationMinutes := len(traces.TotalInvocationsPerMinute)

	minute := 0
	wg2.Add(1)
	go func() {
		f, err := os.OpenFile("output.csv", os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		if err != nil {
			wg2.Done()
			panic(err)
		}
		f.WriteString("millisecond,functionHash,runtime,memory,maxMemory,cpu,maxCpu\n")

		for msg := range writer {
			if msg == "done" {
				break
			}
			f.WriteString(msg)
		}
		fmt.Println("Writer done")
		wg2.Done()
		f.Close()
	}()

	for ; minute < int(totalDurationMinutes); minute++ {
		log.Infof("Processing minute %d", minute)
		var iats []float64
		var numFuncInvokedThisMinute int64 = 0

		rps := int(math.Ceil(float64(traces.TotalInvocationsPerMinute[minute]) / 60.0))
		numInvocatonsThisMinute := traces.TotalInvocationsPerMinute[minute]
		iats = gen.GenerateOneMinuteInterarrivalTimesInMicro(
			numInvocatonsThisMinute,
			gen.Poisson,
		)
		if numInvocatonsThisMinute < 1 {
			continue
		} else {
			wg.Add(1)
			go func(minute int, iats []float64) {
				log.Infof("Minute[%d]\t RPS=%d", minute, rps)
				for nxt := 0; nxt < numInvocatonsThisMinute; nxt++ {
					var runtimeRequested, memoryRequested int
					sum := 0.0
					for i := 0; i <= nxt; i++ {
						sum += iats[i]
					}

					tt := time.Duration(minute)*time.Minute + time.Duration(sum)*time.Microsecond

					atomic.AddInt64(&numFuncInvokedThisMinute, 1)
					funcIndx := invocationsEachMinute[minute][nxt]
					function := traces.Functions[funcIndx]
					runtimeRequested, memoryRequested = gen.GenerateExecutionSpecs(function)
					writer <- fmt.Sprintf("%v,%v,%v,%v,%v,%v,%v\n", int(tt/time.Millisecond), function.HashApp, runtimeRequested, memoryRequested, function.MemoryStats.Percentile100, convertMemoryToCPU(memoryRequested), convertMemoryToCPU(function.MemoryStats.Percentile100))
				}
				log.Info("Minute[", minute, "]\t", "exited")
				wg.Done()
			}(minute, iats)
		}
	}
	log.Info("Waiting for all goroutines to finish")
	wg.Wait()
	writer <- "done"
	wg2.Wait()
}
