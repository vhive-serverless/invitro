package main

import (
	"flag"
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/driver"
	"os"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	traces  common.FunctionTraces
	iatType common.IatDistribution

	yamlSpecificationPath = ""

	seed      = flag.Int64("seed", 42, "Random seed for the function specification generator")
	verbosity = flag.String("verbosity", "all", "Logging verbosity - choose from [all, debug, info]")

	yamlSelector = flag.String("yaml", "trace", "Choose a function YAML from [wimpy, trace, trace_firecracker]")
	endpointPort = flag.Int("endpointPort", 80, "Port to append to an endpoint URL")

	iatDistribution = flag.String("iatDistribution", "exponential", "Choose IAT distribution from [exponential, uniform, equidistant]")
	tracePath       = flag.String("tracePath", "data/traces/", "Path to folder where the trace is located")

	cluster  = flag.Int("cluster", 1, "Size of the cluster measured by #workers")
	duration = flag.Int("duration", 1, "Duration of the experiment in minutes")

	isPartiallyPanic         = flag.Bool("partiallyPanic", false, "Enable partially panic mode in Knative")
	enableWarmupAndProfiling = flag.Bool("warmupAndProfiling", false, "Enable trace profiling and warmup")
	enableTracing            = flag.Bool("enableTracing", false, "Embed loader spans into Zipkin tracing")
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
		log.Debug("All messages will be logged out")
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging has been enabled")
	case "info":
		log.SetLevel(log.InfoLevel)
		log.Debug("Info logging has been enabled")
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

	invocationPath := *tracePath + "invocations.csv"
	runtimePath := *tracePath + "runtime.csv"
	memoryPath := *tracePath + "memory.csv"

	runTraceMode(invocationPath, runtimePath, memoryPath)
}

func runTraceMode(invocationPath, runtimePath, memoryPath string) {
	/** Trace parsing */
	if *duration < 1 {
		log.Fatal("Trace duration should be longer than 0 minutes.")
	}

	amendedDuration := *duration
	if *enableWarmupAndProfiling {
		amendedDuration += common.WARMUP_DURATION_MINUTES * 2
	}

	traces = tc.ParseInvocationTrace(invocationPath, common.MinOf(1440, amendedDuration))
	tc.ParseDurationTrace(&traces, runtimePath)
	tc.ParseMemoryTrace(&traces, memoryPath)

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Printf("\t%s\n", function.Name)
	}

	totalNumPhases := 2

	/* Profiling */
	if *enableWarmupAndProfiling {
		for funcIdx := 0; funcIdx < len(traces.Functions); funcIdx++ {
			tc.ProfileFunction(traces.Functions[funcIdx], common.PROFILING_DURATION_MINUTES)
		}
		traces.WarmupScales = driver.ComputeFunctionWarmupScales(*cluster, traces.Functions)
	}

	/** Deployment */
	driver.DeployFunctions(traces.Functions, yamlSpecificationPath, traces.WarmupScales, *isPartiallyPanic, *endpointPort)

	/** Warmup (Phase 1) */
	nextPhaseStart := 0
	if *enableWarmupAndProfiling {
		nextPhaseStart = driver.Warmup(totalNumPhases, traces.Functions, traces, iatType, *enableTracing, *seed)
	}

	/** Measurement (Phase 2) */
	if nextPhaseStart == *duration {
		// gen.DumpOverloadFlag()
		log.Warnf("Warmup failed to finish in %d minutes", *duration)
	}

	log.Infof("Phase 2 - Generate real workloads")

	traceLoadParams := &driver.DriverConfiguration{
		Functions:                     traces.Functions,
		TotalNumInvocationsEachMinute: traces.TotalInvocationsPerMinute[nextPhaseStart : nextPhaseStart+*duration],
		IATDistribution:               iatType,
		WithTracing:                   *enableTracing,
		Seed:                          *seed,
	}
	driver.NewDriver(traceLoadParams).RunExperiment()
}
