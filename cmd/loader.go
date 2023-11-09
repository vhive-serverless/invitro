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
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/pkg/driver"
	"github.com/vhive-serverless/loader/pkg/generator"
	"github.com/vhive-serverless/loader/pkg/trace"
	tracer "github.com/vhive-serverless/vSwarm/utils/tracing/go"
	"golang.org/x/exp/slices"
	"math/rand"
	"strconv"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	configPath    = flag.String("config", "cmd/config_knative_trace.json", "Path to loader configuration file")
	failurePath   = flag.String("failureConfig", "cmd/failure.json", "Path to the failure configuration file")
	verbosity     = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	iatGeneration = flag.Bool("iatGeneration", false, "Generate iats only or run invocations as well")
	iatFromFile   = flag.Bool("generated", false, "True if iats were already generated")
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

	if *iatGeneration {
		durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
		iatDistribution, shiftIAT := parseIATDistribution(&cfg)
		traceParser := trace.NewAzureParser(cfg.TracePath, durationToParse)
		functions := traceParser.Parse(cfg.Platform)

		justGenerateIAT(cfg.Seed, iatDistribution, shiftIAT, parseTraceGranularity(&cfg), functions)
	}

	supportedPlatforms := []string{
		"Knative",
		"Knative-RPS",
		"OpenWhisk",
		"OpenWhisk-RPS",
		"AWSLambda",
		"AWSLambda-RPS",
		"Dirigent",
		"Dirigent-RPS",
		"Dirigent-Dandelion-RPS",
		"Dirigent-Dandelion",
	}

	if !slices.Contains(supportedPlatforms, cfg.Platform) {
		log.Fatal("Unsupported platform!")
	}

	if !strings.HasSuffix(cfg.Platform, "-RPS") {
		runTraceMode(&cfg, *iatFromFile, *iatGeneration)
	} else {
		runRPSMode(&cfg, *iatFromFile, *iatGeneration)
	}
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

func parseIATDistribution(cfg *config.LoaderConfiguration) (common.IatDistribution, bool) {
	switch cfg.IATDistribution {
	case "exponential":
		return common.Exponential, false
	case "exponential_shift":
		return common.Exponential, true
	case "uniform":
		return common.Uniform, false
	case "uniform_shift":
		return common.Uniform, true
	case "equidistant":
		return common.Equidistant, false
	default:
		log.Fatal("Unsupported IAT distribution.")
	}

	return common.Exponential, false
}

func parseYAMLSpecification(cfg *config.LoaderConfiguration) string {
	switch cfg.YAMLSelector {
	case "container":
		return "workloads/container/trace_func_go.yaml"
	case "firecracker":
		return "workloads/firecracker/trace_func_go.yaml"
	default:
		if cfg.Platform != "Dirigent" && cfg.Platform != "Dirigent-RPS" && cfg.Platform != "Dirigent-Dandelion-RPS" && cfg.Platform != "Dirigent-Dandelion" {
			log.Fatal("Invalid 'YAMLSelector' parameter.")
		}
	}

	return ""
}

func parseTraceGranularity(cfg *config.LoaderConfiguration) common.TraceGranularity {
	switch cfg.Granularity {
	case "minute":
		return common.MinuteGranularity
	case "second":
		return common.SecondGranularity
	default:
		log.Fatal("Invalid trace granularity parameter.")
	}

	return common.MinuteGranularity
}

func runTraceMode(cfg *config.LoaderConfiguration, readIATFromFile bool, justGenerateIAT bool) {
	durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)

	traceParser := trace.NewAzureParser(cfg.TracePath, durationToParse)
	functions := traceParser.Parse(cfg.Platform)

	log.Infof("Traces contain the following %d functions:\n", len(functions))
	for _, function := range functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	iatType, shiftIAT := parseIATDistribution(cfg)

	log.Infof("Using %s as a service YAML specification file.\n", yamlPath)

	experimentDriver := driver.NewDriver(&config.Configuration{
		LoaderConfiguration:  cfg,
		FailureConfiguration: config.ReadFailureConfiguration(*failurePath),

		IATDistribution:  iatType,
		ShiftIAT:         shiftIAT,
		TraceGranularity: parseTraceGranularity(cfg),
		TraceDuration:    durationToParse,

		YAMLPath: yamlPath,
		TestMode: false,

		Functions: functions,
	})

	experimentDriver.RunExperiment(justGenerateIAT, readIATFromFile)
}

func justGenerateIAT(seed int64, iatDistribution common.IatDistribution, shiftIAT bool, traceGranularity common.TraceGranularity, functions []*common.Function) {
	specificationGenerator := generator.NewSpecificationGenerator(seed)

	for i, function := range functions {
		spec := specificationGenerator.GenerateInvocationData(
			function,
			iatDistribution,
			shiftIAT,
			traceGranularity,
		)
		functions[i].Specification = spec

		file, _ := json.MarshalIndent(spec, "", " ")
		err := os.WriteFile("iat"+strconv.Itoa(i)+".json", file, 0644)
		if err != nil {
			log.Fatalf("Writing the loader config file failed: %s", err)
		}
	}
}

func runRPSMode(cfg *config.LoaderConfiguration, readIATFromFile bool, justGenerateIAT bool) {
	rpsTarget := cfg.RpsTarget
	coldStartPercentage := cfg.RpsColdStartRatioPercentage

	warmStartRPS := rpsTarget * (100 - coldStartPercentage) / 100
	coldStartRPS := rpsTarget * coldStartPercentage / 100

	warmFunction, warmStartCount := generator.GenerateWarmStartFunction(cfg.ExperimentDuration, warmStartRPS)
	coldFunctions, coldStartCount := generator.GenerateColdStartFunctions(cfg.ExperimentDuration, coldStartRPS, cfg.CooldownSeconds)

	experimentDriver := driver.NewDriver(&config.Configuration{
		LoaderConfiguration: cfg,
		TraceDuration:       determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration),

		Functions: createRPSFunctions(cfg, warmFunction, warmStartCount, coldFunctions, coldStartCount),
	})

	experimentDriver.RunExperiment(justGenerateIAT, readIATFromFile)
}

func createRPSFunctions(cfg *config.LoaderConfiguration, warmFunction common.IATArray, warmFunctionCount []int,
	coldFunctions []common.IATArray, coldFunctionCount [][]int) []*common.Function {
	var result []*common.Function

	result = append(result, &common.Function{
		Name: fmt.Sprintf("warm-function-%d", rand.Int()),

		InvocationStats: &common.FunctionInvocationStats{Invocations: warmFunctionCount},
		MemoryStats:     &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
		DirigentMetadata: &common.DirigentMetadata{
			Image:               "trace",
			Port:                80,
			Protocol:            "tcp",
			ScalingUpperBound:   256,
			ScalingLowerBound:   1,
			IterationMultiplier: cfg.RpsIterationMultiplier,
		},

		Specification: &common.FunctionSpecification{
			IAT:                  warmFunction,
			RuntimeSpecification: createRuntimeSpecification(len(warmFunction), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
		},
	})

	for i := 0; i < len(coldFunctions); i++ {
		result = append(result, &common.Function{
			Name: fmt.Sprintf("cold-function-%d-%d", i, rand.Int()),

			InvocationStats: &common.FunctionInvocationStats{Invocations: coldFunctionCount[i]},
			MemoryStats:     &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
			DirigentMetadata: &common.DirigentMetadata{
				Image:               "trace",
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1,
				ScalingLowerBound:   0,
				IterationMultiplier: cfg.RpsIterationMultiplier,
			},

			Specification: &common.FunctionSpecification{
				IAT:                  coldFunctions[i],
				RuntimeSpecification: createRuntimeSpecification(len(coldFunctions[i]), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},
		})
	}

	return result
}

func createRuntimeSpecification(count int, runtime, memory int) common.RuntimeSpecificationArray {
	var result common.RuntimeSpecificationArray
	for i := 0; i < count; i++ {
		result = append(result, common.RuntimeSpecification{
			Runtime: runtime,
			Memory:  memory,
		})
	}

	return result
}
