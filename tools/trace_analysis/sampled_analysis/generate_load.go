package main

import (
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"

	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
)

var (
	invPath    = flag.String("invPath", "../../../data/traces/traces/1500_inv.csv", "path to invocation trace")
	runPath    = flag.String("runPath", "../../../data/traces/traces/1500_run.csv", "path to runtime trace")
	memPath    = flag.String("memPath", "../../../data/traces/traces/1500_mem.csv", "path to memory trace")
	outputFile = flag.String("outputFile", "output.csv", "name of output file")
)

func main() {
	flag.Parse()
	var wg sync.WaitGroup
	var wg2 sync.WaitGroup

	gen.InitSeed(42)
	writer := make(chan string)

	var traces tc.FunctionTraces

	traces = tc.ParseInvocationTrace(*invPath, 1440)
	tc.ParseDurationTrace(&traces, *runPath)
	tc.ParseMemoryTrace(&traces, *memPath)

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")

	invocationsEachMinute := traces.InvocationsEachMinute[:]
	gen.ShuffleAllInvocationsInplace(&invocationsEachMinute)

	totalDurationMinutes := len(traces.TotalInvocationsPerMinute)

	wg2.Add(1)
	go func() {
		f, err := os.OpenFile(*outputFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
		if err != nil {
			wg2.Done()
			panic(err)
		}
		f.WriteString("millisecond,functionHash,runtime,memory,maxMemory,cpu,maxCpu\n")

		for msg := range writer {
			f.WriteString(msg)
		}
		wg2.Done()
		f.Close()
	}()

	for minute := 0; minute < int(totalDurationMinutes); minute++ {
		var iats []float64
		var numFuncInvokedThisMinute int64 = 0

		numInvocatonsThisMinute := traces.TotalInvocationsPerMinute[minute]
		iats = gen.GenerateOneMinuteInterarrivalTimesInMicro(
			numInvocatonsThisMinute,
			gen.Poisson,
		)
		if numInvocatonsThisMinute < 1 {
			continue
		}
		wg.Add(1)
		go func(minute int, iats []float64) {
			for nxt := 0; nxt < numInvocatonsThisMinute; nxt++ {
				var runtimeRequested, memoryRequested int
				var sum float64

				for i := 0; i <= nxt; i++ {
					sum += iats[i]
				}

				tt := time.Duration(minute)*time.Minute + time.Duration(sum)*time.Microsecond

				atomic.AddInt64(&numFuncInvokedThisMinute, 1)
				funcIndx := invocationsEachMinute[minute][nxt]
				function := traces.Functions[funcIndx]
				runtimeRequested, memoryRequested = gen.GenerateExecutionSpecs(function)
				writer <- fmt.Sprintf("%v,%v,%v,%v,%v,%v,%v\n", int(tt/time.Millisecond), function.HashApp, runtimeRequested, memoryRequested, function.MemoryStats.Percentile100, tc.ConvertMemoryToCpu(memoryRequested), tc.ConvertMemoryToCpu(function.MemoryStats.Percentile100))
			}
			wg.Done()
		}(minute, iats)
	}
	wg.Wait()
	close(writer)
	wg2.Wait()
}
