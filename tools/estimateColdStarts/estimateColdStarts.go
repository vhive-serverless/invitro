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
	"os"
	"slices"
	"sync"
	"time"

	"github.com/gocarina/gocsv"
	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/common"
	spec "github.com/vhive-serverless/loader/pkg/generator"
	trace "github.com/vhive-serverless/loader/pkg/trace"
)

var (
	tracePath       = flag.String("tracePath", "data/traces/", "Path to folder where the trace is located")
	outputFile      = flag.String("outputFile", "output.csv", "Path to output file")
	duration        = flag.Int("duration", 1440, "Duration of the traces in minutes")
	iatDistribution = flag.String("iatDistribution", "exponential", "IAT distribution, one of [exponential(_shift), uniform(_shift), equidistant(_shift)]")
	randSeed        = flag.Uint64("randSeed", 42, "Seed for the random number generator")
	keepalive       = flag.Int("keepalive", 6, "Keepalive period in seconds")
)

type coldStartRecord struct {
	Timestamp   int `csv:"timestamp"`
	FunctionNum int `csv:"functionNum"`
}

func main() {
	flag.Parse()

	coldStarts(*outputFile)
}

func coldStarts(outputFilename string) {
	var wg sync.WaitGroup
	var wg2 sync.WaitGroup

	var iatType common.IatDistribution
	shift := false
	switch *iatDistribution {
	case "exponential":
		iatType = common.Exponential
	case "exponential_shift":
		iatType = common.Exponential
		shift = true
	case "gamma":
		iatType = common.Gamma
	case "gamma_shift":
		iatType = common.Gamma
		shift = true
	case "uniform":
		iatType = common.Uniform
	case "uniform_shift":
		iatType = common.Uniform
		shift = true
	case "equidistant":
		iatType = common.Equidistant
	default:
		log.Fatal("Unsupported IAT distribution.")
	}
	writer := make(chan interface{}, 1000)

	traceParser := trace.NewAzureParser(*tracePath, *duration)
	functions := traceParser.Parse("Knative")

	log.Infof("Traces contain the following %d functions:\n", len(functions))

	wg2.Add(1)
	go func() {
		defer wg2.Done()
		f, err := os.Create(outputFilename)
		if err != nil {
			log.Fatal(err)
		}
		_ = gocsv.MarshalChan(writer, gocsv.DefaultCSVWriter(f))
		f.Close()
	}()

	specGenerator := spec.NewSpecificationGenerator(*randSeed)

	for i, function := range functions {
		spec := specGenerator.GenerateInvocationData(function, iatType, shift, common.MinuteGranularity)
		functions[i].Specification = spec
	}

	limiter := make(chan struct{}, 12)

	for i, function := range functions {
		wg.Add(1)
		limiter <- struct{}{}
		go generateFunctionTimeline(function, i, writer, &wg, limiter)
	}
	wg.Wait()
	close(writer)
	wg2.Wait()
}

func generateFunctionTimeline(function *common.Function, orderNum int, writer chan interface{}, wg *sync.WaitGroup, limiter chan struct{}) {
	defer wg.Done()
	defer func() { <-limiter }()
	minuteIndex, invocationIndex := 0, 0
	sum := 0.0

	IAT, runtimeSpecification := function.Specification.IAT, function.Specification.RuntimeSpecification

	concurrency := make([]int, *duration*60*1e3+common.MaxExecTimeMilli)

	for {
		if minuteIndex >= *duration {
			break
		} else if function.InvocationStats.Invocations[minuteIndex] == 0 {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
			continue
		}

		sum += IAT[minuteIndex][invocationIndex]

		duration := runtimeSpecification[minuteIndex][invocationIndex].Runtime
		startTime := int((time.Duration(minuteIndex)*time.Minute + time.Duration(sum)*time.Microsecond) / time.Millisecond)
		// log.Infof("Function %s, order %d, minute %d, invocation %d, start time %d, duration %d", function.Name, orderNum, minuteIndex, invocationIndex, startTime, duration)
		for i := startTime; i <= startTime+duration; i++ {
			concurrency[i]++
		}
		// log.Infof("%v", concurrency[startTime:startTime+duration])

		invocationIndex++
		if function.InvocationStats.Invocations[minuteIndex] == invocationIndex {
			minuteIndex++
			invocationIndex = 0
			sum = 0.0
		}
	}

	capacity := 0
	for i, c := range concurrency {
		if i == 0 {
			capacity = 0
		} else if c <= concurrency[i-1] {
			continue
		} else {
			capacity = slices.Max(concurrency[max(0, i-*keepalive*1e3):i])
		}
		for ; capacity < c; capacity++ {
			writer <- &coldStartRecord{
				Timestamp:   i,
				FunctionNum: orderNum,
			}
		}
	}
}
