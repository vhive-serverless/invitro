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
	"os"
	"time"

	"github.com/vhive-serverless/loader/pkg/generator"

	"golang.org/x/exp/slices"

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
	configPath    = flag.String("config", "cmd/config_knative_trace.json", "Path to loader configuration file")
	failurePath   = flag.String("failureConfig", "cmd/failure.json", "Path to the failure configuration file")
	verbosity     = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	iatGeneration = flag.Bool("iatGeneration", false, "Generate IATs only or run invocations as well")
	iatFromFile   = flag.Bool("generated", false, "True if iats were already generated")
	dryRun        = flag.Bool("dryRun", false, "Dry run mode - do not deploy functions or generate invocations")
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
		common.PlatformKnative,
		common.PlatformOpenWhisk,
		common.PlatformAWSLambda,
		common.PlatformDirigent,
		common.PlatformAzureFunctions,
	}
	if !slices.Contains(supportedPlatforms, cfg.Platform) {
		log.Fatal("Unsupported platform!")
	}

	if cfg.Platform == common.PlatformKnative {
		common.CheckCPULimit(cfg.CPULimit)
	}

	run(&cfg, *iatFromFile, *iatGeneration)
}

func determineDurationToParse(runtimeDuration int, warmupDuration int) int {
	result := 0

	if warmupDuration > 0 {
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

// Return YAML for "container" or "firecracker microVM"
func parseYAMLSpecification(cfg *config.LoaderConfiguration) string {
	switch cfg.YAMLSelector {
	case "container":
		return "workloads/container/trace_func_go.yaml"
	case "firecracker":
		return "workloads/firecracker/trace_func_go.yaml"
	default:
		if cfg.Platform != common.PlatformDirigent && cfg.Platform != common.PlatformAzureFunctions {
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

// Determine if pathString is Azure2019 directory, or Azure2021 .csv file
func determine2019Or2021(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		log.Fatalf("Unable to determine path string '%s', Error: %s", path, err)
		return "err"
	} else {
		// Assume is Azure2021 .csv file
		if info.Mode().IsRegular() {
			return "Azure2021"
		}
		// Assume is Azure2019 directory
		if info.IsDir() {
			return "Azure2019"
		}

		log.Fatalf("Path string '%s' is not file or directory", path)
		return "err"
	}
}

func run(cfg *config.LoaderConfiguration, readIATFromFile bool, writeIATsToFile bool) {
	//
	// Determine type of input.
	//
	var traceInputType string
	if cfg.TracePath == "RPS" {
		traceInputType = "RPS"
	} else {
		if cfg.VSwarm {
			traceInputType = "vSwarm"
		} else if !cfg.VSwarm {
			traceInputType = determine2019Or2021(cfg.TracePath)
		} else { // Reduant, for future input types.
			log.Fatal("Unsupported Trace Input Type", traceInputType)
		}
	}
	log.Info("Detected Trace Input Type", traceInputType)

	//
	// Generate common.Functions + FunctionSpecification (Function's deployment and invocation info)
	//
	var functions []*common.Function
	switch traceInputType {
	case "RPS":
		functions = RPSGenerateFunctions(cfg)
	case "Azure2019", "vSwarm":
		functions = Azure2019GenerateFunctions(cfg)
	case "Azure2021":
		functions = Azure2021GenerateFunctions(cfg)
	}

	//
	// Handle Dirgient (Container orchestrator)
	// Reads Dirigent trace meta-data, and places into *common.Function as property "dirigentMetadata"
	//
	dirigentConfig := config.ReadDirigentConfig(cfg)
	switch traceInputType {
	case "RPS":
		generator.AppendDirigentMetadata(functions, cfg, dirigentConfig)
	case "Azure2019", "vSwarm", "Azure2021":
		yamlPath := parseYAMLSpecification(cfg)
		dirigentMetadataParser := trace.NewDirigentMetadataParser(cfg.TracePath, functions, yamlPath, cfg.Platform)
		dirigentMetadataParser.Parse()
		// TODO, confirm if dirigent handling will work seamlessly.
	}

	log.Infof("Traces contain the following %d functions:\n", len(functions))
	for _, function := range functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	driver.ReadOrWriteSpecificationToFile(functions, writeIATsToFile, readIATFromFile)

	experimentDuration := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	experimentDriver := driver.NewDriver(&config.Configuration{
		LoaderConfiguration:   cfg,
		FailureConfiguration:  config.ReadFailureConfiguration(*failurePath),
		DirigentConfiguration: dirigentConfig,

		TraceGranularity: parseTraceGranularity(cfg),
		TestMode:         false,

		TraceDuration: experimentDuration,
		Functions:     functions,
	})

	// Skip experiments execution during dry run mode
	if *dryRun {
		return
	}

	experimentDriver.RunExperiment()
}

func RPSGenerateFunctions(cfg *config.LoaderConfiguration) []*common.Function {
	experimentDuration := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)

	rpsTarget := cfg.RpsTarget
	coldStartPercentage := cfg.RpsColdStartRatioPercentage

	warmStartRPS := rpsTarget * (100 - coldStartPercentage) / 100
	coldStartRPS := rpsTarget * coldStartPercentage / 100

	// IAT, PerMinuteCount
	warmFunction, warmStartCount := generator.GenerateWarmStartFunction(experimentDuration, warmStartRPS)
	coldFunctions, coldStartCount := generator.GenerateColdStartFunctions(experimentDuration, coldStartRPS, cfg.RpsCooldownSeconds)

	functions := generator.CreateRPSFunctions(cfg, warmFunction, warmStartCount, coldFunctions, coldStartCount, yamlPath)

	return functions
}

func Azure2019GenerateFunctions(cfg *config.LoaderConfiguration) []*common.Function {
	durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)
	var functions []*common.Function
	var traceParser trace.Parser

	// Azure trace parsing
	if !cfg.VSwarm {
		traceParser = trace.NewAzureParser(cfg.TracePath, durationToParse, yamlPath)
	} else {
		traceParser = trace.NewMapperParser(cfg.TracePath, durationToParse)
	}
	functions = traceParser.Parse()

	iatType, shiftIAT := parseIATDistribution(cfg)
	traceGranularity := parseTraceGranularity(cfg)
	generator.GenerateAzure2019Specification(functions, cfg, iatType, shiftIAT, traceGranularity)

	return functions
}

func Azure2021GenerateFunctions(cfg *config.LoaderConfiguration) []*common.Function {
	durationToParse := determineDurationToParse(cfg.ExperimentDuration, cfg.WarmupDuration)
	yamlPath := parseYAMLSpecification(cfg)

	traceParser := trace.NewAzure2021Parser(cfg.TracePath, durationToParse, yamlPath)
	functions := traceParser.Parse()

	return functions
}
