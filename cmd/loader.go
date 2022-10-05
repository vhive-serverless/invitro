package main

import (
	"flag"
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/driver"
	"github.com/eth-easl/loader/pkg/trace"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	iatType               common.IatDistribution
	yamlSpecificationPath = ""

	seed      = flag.Int64("seed", 42, "Random seed for the function specification generator")
	verbosity = flag.String("verbosity", "all", "Logging verbosity - choose from [all, debug, info]")

	yamlSelector = flag.String("yaml", "trace", "Choose a function YAML from [wimpy, trace, trace_firecracker]")
	endpointPort = flag.Int("endpointPort", 80, "Port to append to an endpoint URL")

	iatDistribution = flag.String("iatDistribution", "exponential", "Choose IAT distribution from [exponential, uniform, equidistant]")
	tracePath       = flag.String("tracePath", "data/traces/", "Path to folder where the trace is located")

	duration = flag.Int("duration", 1, "Duration of the experiment in minutes")

	isPartiallyPanic         = flag.Bool("partiallyPanic", false, "Enable partially panic mode in Knative")
	enableWarmupAndProfiling = flag.Bool("warmup", false, "Enable trace profiling and warmup")
	enableTracing            = flag.Bool("enableTracing", false, "Embed loader spans into Zipkin tracing")
	enableMetrics            = flag.Bool("enableMetrics", false, "Enable metrics scrapping from the cluster")
)

func init() {
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	switch *verbosity {
	case "all":
		log.SetLevel(log.TraceLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	}

	switch *yamlSelector {
	case "wimpy":
		yamlSpecificationPath = "workloads/container/wimpy.yaml"
	case "trace":
		yamlSpecificationPath = "workloads/container/trace_func_go.yaml"
	case "trace_firecracker":
		yamlSpecificationPath = "workloads/firecracker/trace_func_go.yaml"
	}

	log.Infof("Using %s as a service YAML specification file.\n", yamlSpecificationPath)
}

func main() {
	if *enableTracing {
		// TODO: how not to exclude Zipkin spans here? - file a feature request
		log.Warnf("Zipkin tracing has been enabled. This will exclude Istio spans from the Zipkin traces.")

		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "loader")
		if err != nil {
			log.Print(err)
		}

		defer shutdown()
	}

	if *duration < 1 {
		log.Fatal("Runtime duration should be longer at least a minute.")
	}

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

	runTraceMode()
}

func determineDurationToParse(runtimeDuration int, withWarmup bool) int {
	result := 0

	if withWarmup {
		result += 1                              // profiling
		result += common.WARMUP_DURATION_MINUTES // warmup
	}

	result += runtimeDuration // actual experiment

	return result
}

func runTraceMode() {
	durationToParse := determineDurationToParse(*duration, *enableWarmupAndProfiling)

	traceParser := trace.NewAzureParser(*tracePath, durationToParse)
	functions := traceParser.Parse()

	log.Infof("Traces contain the following %d functions:\n", len(functions))
	for _, function := range functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	experimentDriver := driver.NewDriver(&driver.DriverConfiguration{
		EnableMetricsCollection: *enableMetrics,
		IATDistribution:         iatType,
		PathToTrace:             *tracePath,
		TraceDuration:           durationToParse,

		YAMLPath:         yamlSpecificationPath,
		IsPartiallyPanic: *isPartiallyPanic,
		EndpointPort:     *endpointPort,

		WithTracing: *enableTracing,
		WithWarmup:  *enableWarmupAndProfiling,
		Seed:        *seed,
		TestMode:    false,

		Functions: functions,
	})

	experimentDriver.RunExperiment()
}
