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
	"fmt"
	"golang.org/x/exp/slices"
	"os"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver"
	"github.com/vhive-serverless/loader/pkg/trace"

	log "github.com/sirupsen/logrus"

	tracer "github.com/vhive-serverless/vSwarm/utils/tracing/go"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	configPath    = flag.String("config", "config.json", "Path to loader configuration file")
	verbosity     = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	iatGeneration = flag.Bool("iatGeneration", false, "Generate iats only or run invocations as well")
	generated     = flag.Bool("generated", false, "True if iats were already generated")
)

func init() {
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	switch *verbosity {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "trace":
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

func main() {
	cfg := config.ReadConfigurationFile(*configPath)

	if cfg.EnableZipkinTracing {
		// TODO: how not to exclude Zipkin spans here? - file a feature request
		log.Warnf("Zipkin tracing has been enabled. This will exclude Istio spans from the Zipkin traces.")

		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "loader")
		if err != nil {
			log.Print(err)
		}

		defer shutdown()
	}

	if cfg.ExperimentDuration < 1 {
		log.Fatal("Runtime duration should be longer, at least a minute.")
	}

	supportedPlatforms := []string{
		"Knative",
		"OpenWhisk",
		"AWSLambda",
		"Dirigent",
	}

	if !slices.Contains(supportedPlatforms, cfg.Platform) {
		log.Fatal("Unsupported platform! Supported platforms are [Knative, OpenWhisk, AWSLambda, Dirigent]")
	}

	runTraceMode(&cfg, *iatGeneration, *generated)
}

func determineDurationToParse(runtimeDuration int, warmupDuration int) int {
	result := 0

	if warmupDuration > 0 {
		result += 1              // profiling
		result += warmupDuration // warmup
	}

	result += runtimeDuration // actual experiment

	return result
}

func runTraceMode(cfg *config.LoaderConfiguration, iatOnly bool, generated bool) {
	durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)

	traceParser := trace.NewAzureParser(cfg.TracePath, durationToParse)
	functions := traceParser.Parse(cfg.Platform)

	log.Infof("Traces contain the following %d functions:\n", len(functions))
	for _, function := range functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	var iatType common.IatDistribution
	shiftIAT := false
	switch cfg.IATDistribution {
	case "exponential":
		iatType = common.Exponential
	case "exponential_shift":
		iatType = common.Exponential
		shiftIAT = true
	case "uniform":
		iatType = common.Uniform
	case "uniform_shift":
		iatType = common.Uniform
		shiftIAT = true
	case "equidistant":
		iatType = common.Equidistant
	default:
		log.Fatal("Unsupported IAT distribution.")
	}

	var yamlSpecificationPath string
	switch cfg.YAMLSelector {
	case "wimpy":
		yamlSpecificationPath = "workloads/container/wimpy.yaml"
	case "container":
		yamlSpecificationPath = "workloads/container/trace_func_go.yaml"
	case "firecracker":
		yamlSpecificationPath = "workloads/firecracker/trace_func_go.yaml"
	case "kwok":
		yamlSpecificationPath = "workloads/container/kwok_fake_pod.yaml"
	default:
		if cfg.Platform != "Dirigent" {
			log.Fatal("Invalid 'YAMLSelector' parameter.")
		}
	}

	var traceGranularity common.TraceGranularity
	switch cfg.Granularity {
	case "minute":
		traceGranularity = common.MinuteGranularity
	case "second":
		traceGranularity = common.SecondGranularity
	default:
		log.Fatal("Invalid trace granularity parameter.")
	}

	log.Infof("Using %s as a service YAML specification file.\n", yamlSpecificationPath)

	experimentDriver := driver.NewDriver(&driver.DriverConfiguration{
		LoaderConfiguration: cfg,
		IATDistribution:     iatType,
		ShiftIAT:            shiftIAT,
		TraceGranularity:    traceGranularity,
		TraceDuration:       durationToParse,

		YAMLPath: yamlSpecificationPath,
		TestMode: false,

		Functions: functions,
	})

	experimentDriver.RunExperiment(iatOnly, generated)
}
