/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package main

import (
	"flag"
	"io"
	"os"
	"sync"
	"time"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/common"
	spec "github.com/vhive-serverless/loader/pkg/generator"
	trace "github.com/vhive-serverless/loader/pkg/trace"
)

var (
	scale           = flag.String("scale", "millisecond", "Scale of the timeline to generate, one of [millisecond, minute]")
	tracePath       = flag.String("tracePath", "data/traces/", "Path to folder where the trace is located")
	outputFile      = flag.String("outputFile", "output.csv", "Path to output file")
	duration        = flag.Int("duration", 1440, "Duration of the traces in minutes")
	cpuQuota        = flag.Bool("cpuQuota", true, "Whether to use the CPU quota or not")
	iatDistribution = flag.String("iatDistribution", "exponential", "IAT distribution, one of [exponential, uniform, equidistant]")
	randSeed        = flag.Int64("randSeed", 42, "Seed for the random number generator")
)

type loaderRecord struct {
	Millisecond  int    `csv:"millisecond"`
	FunctionHash string `csv:"functionHash"`
	Runtime      int    `csv:"runtime"`
	Memory       int    `csv:"memory"`
	MemoryUsage  int    `csv:"maxMemory"`
	Cpu          int    `csv:"cpu"`
}

type minuteTimelineRecord struct {
	Minute         int     `csv:"minute"`
	FunctionHash   string  `csv:"functionHash"`
	AvgRuntime     float64 `csv:"avgRuntime"`
	AvgMemory      float64 `csv:"avgMemory"`
	AvgCpu         float64 `csv:"avgCpu"`
	AvgMemoryUsage float64 `csv:"avgMemoryUsage"`
}

type timelineRecord struct {
	Timestamp   int `csv:"timestamp"`
	FuncCnt     int `csv:"funcCnt"`
	MemoryUsage int `csv:"memoryUsage"`
	Memory      int `csv:"memory"`
	Cpu         int `csv:"cpu"`
}

func main() {
	flag.Parse()
	if *scale != "millisecond" && *scale != "minute" {
		log.Fatal("Invalid scale: ", *scale, ", must be one of [millisecond, minute]")
	}

	if *scale == "minute" {
		generateLoad(*outputFile, false)
		return
	}

	generateLoad("tmp_"+*outputFile, true)
	log.Info("Generated Load, Building timeline")
	buildTimeline("tmp_"+*outputFile, *outputFile)

}

func generateLoad(outputFilename string, millisecondScale bool) {
	var wg sync.WaitGroup
	var wg2 sync.WaitGroup

	var iatType common.IatDistribution
	switch *iatDistribution {
	case "exponential":
		iatType = common.Exponential
	case "uniform":
		iatType = common.Uniform
	case "equidistant":
		iatType = common.Equidistant
	default:
		log.Fatal("Unsupported IAT distribution.")
	}
	writer := make(chan any, 1000)

	traceParser := trace.NewAzureParser(*tracePath, *duration, "")
	functions := traceParser.Parse()

	log.Infof("Traces contain the following %d functions:\n", len(functions))

	wg2.Go(func() {
		f, err := os.Create(outputFilename)
		if err != nil {
			log.Fatal(err)
		}
		_ = gocsv.MarshalChan(writer, gocsv.DefaultCSVWriter(f))
		f.Close()
	})

	specGenerator := spec.NewSpecificationGenerator(*randSeed)

	for i, function := range functions {
		spec := specGenerator.GenerateInvocationData(function, iatType, false, common.MinuteGranularity)
		functions[i].Specification = spec
	}

	for _, function := range functions {
		wg.Add(1)
		go generateFunctionTimeline(function, writer, &wg, millisecondScale)
	}
	wg.Wait()
	close(writer)
	wg2.Wait()
}

func generateFunctionTimeline(function *common.Function, writer chan any, wg *sync.WaitGroup, millisecondScale bool) {
	defer wg.Done()

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification
	invocationCount := len(IAT)

	if invocationCount == 0 {
		return
	}
	if invocationCount != len(runtimeSpecification) {
		log.Fatalf("Mismatched IAT and runtime specification lengths for function %s: %d != %d",
			function.Name, invocationCount, len(runtimeSpecification))
	}

	minuteIndexSearch := common.NewIntervalSearch(function.Specification.PerMinuteCount)
	interval := minuteIndexSearch.SearchInterval(0)
	if interval == nil {
		log.Fatalf("No per-minute interval found for function %s with %d generated invocations",
			function.Name, invocationCount)
	}
	minuteIndexEnd, minuteIndex := interval.End, interval.Value

	iatIndex := 0
	var previousIATSum int64
	runtimes, memory, cpuSum, memoryUsage := 0, 0, 0, 0

	for iatIndex < invocationCount {
		if minuteIndex >= *duration {
			break
		}

		iat := time.Duration(IAT[iatIndex]) * time.Microsecond
		previousIATSum += iat.Microseconds()
		runtimeSpec := runtimeSpecification[iatIndex]

		var duration, cpu int
		if *cpuQuota {
			cpu = trace.ConvertMemoryToCpu(int(function.MemoryStats.Percentile100))
			duration = int((float64(runtimeSpec.Runtime) / float64(cpu)) * 1000)
		} else {
			duration = runtimeSpec.Runtime
			cpu = 1 * 1000
		}

		if millisecondScale {
			// Write the millisecond scale timeline
			writer <- loaderRecord{
				Millisecond:  int((time.Duration(previousIATSum) * time.Microsecond) / time.Millisecond),
				FunctionHash: function.InvocationStats.HashApp,
				Runtime:      duration,
				MemoryUsage:  runtimeSpec.Memory,
				Memory:       int(function.MemoryStats.Percentile100),
				Cpu:          cpu,
			}
		} else {
			// Add the millisecond data to list, to be averaged later
			runtimes += duration
			memoryUsage += runtimeSpec.Memory * duration
			cpuSum += cpu
			memory += int(function.MemoryStats.Percentile100) * duration
		}

		iatIndex++
		if iatIndex > minuteIndexEnd {
			if !millisecondScale {
				// Generated one minute of the trace, write the average
				writer <- minuteTimelineRecord{
					Minute:         minuteIndex,
					FunctionHash:   function.InvocationStats.HashApp,
					AvgRuntime:     float64(runtimes) / float64(time.Millisecond),
					AvgMemory:      float64(memory) / float64(time.Millisecond),
					AvgCpu:         float64(cpuSum) / float64(time.Millisecond),
					AvgMemoryUsage: float64(memoryUsage) / float64(time.Millisecond),
				}
				runtimes, memory, cpuSum, memoryUsage = 0, 0, 0, 0
			}

			interval = minuteIndexSearch.SearchInterval(iatIndex)
			if interval == nil {
				break
			}
			minuteIndexEnd, minuteIndex = interval.End, interval.Value
		}
	}
}

func buildTimeline(inputFilename, outputFilename string) {
	flag.Parse()
	f, err := os.Open(inputFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	var maxTime int

	records := []*loaderRecord{}
	if err := gocsv.UnmarshalFile(f, &records); err != nil {
		log.Fatal(err)
	}

	for _, record := range records {
		start := record.Millisecond
		runtime := record.Runtime
		if start+runtime > maxTime {
			maxTime = start + runtime
		}
	}

	funcCnt := make([]int, maxTime+1)
	memUsg := make([]int, maxTime+1)
	memory := make([]int, maxTime+1)
	cpu := make([]int, maxTime+1)

	for _, record := range records {
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		for i := record.Millisecond; i <= record.Millisecond+record.Runtime; i++ {
			funcCnt[i]++
			memUsg[i] += record.MemoryUsage
			memory[i] += record.Memory
			cpu[i] += record.Cpu
		}
	}

	ff, err := os.Create(outputFilename)
	if err != nil {
		log.Fatal(err)
	}

	timelineRecords := []*timelineRecord{}

	for i := 0; i <= maxTime; i++ {
		timelineRecords = append(timelineRecords, &timelineRecord{
			Timestamp:   i,
			FuncCnt:     funcCnt[i],
			MemoryUsage: memUsg[i],
			Memory:      memory[i],
			Cpu:         cpu[i],
		})
	}
	err = gocsv.MarshalFile(&timelineRecords, ff)
	if err != nil {
		log.Fatal(err)
	}
	ff.Close()
}
