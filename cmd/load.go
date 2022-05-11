package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	wu "github.com/eth-easl/loader/cmd/options"
	util "github.com/eth-easl/loader/pkg"
	fc "github.com/eth-easl/loader/pkg/function"
	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	traces tc.FunctionTraces

	serviceConfigPath = ""
	server            = flag.String("server", "trace", "Choose a function server from [wimpy, trace]")

	mode      = flag.String("mode", "trace", "Choose a mode from [trace, stress, coldstart]")
	tracePath = flag.String("tracePath", "data/traces/", "Path to trace")

	print      = flag.String("print", "all", "Choose a mode from [all, debug, info]")
	cluster    = flag.Int("cluster", 1, "Size of the cluster measured by #workers")
	duration   = flag.Int("duration", 3, "Duration of the experiment")
	sampleSize = flag.Int("sample", 10, "Sample size of the traces")

	rps            = flag.Int("rps", -900_000, "Request per second")
	rpsStart       = flag.Int("start", 0, "Starting RPS value")
	rpsEnd         = flag.Int("end", -900_000, "Ending RPS value")
	rpsSlot        = flag.Int("slot", 60, "Time slot in seconds for each RPS in the `stress` mode")
	rpsStep        = flag.Int("step", 1, "Step size for increasing RPS in the `stress` mode")
	totalFunctions = flag.Int("totalFunctions", 1, "Total number of functions used in the `stress` mode")

	seed = flag.Int64("seed", 42, "Random seed for the generator")

	// withWarmup = flag.Int("withWarmup", -1000, "Duration of the withWarmup")
	withWarmup  = flag.Bool("warmup", false, "Enable warmup")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
)

func init() {
	/** Logging. */
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	switch *print {
	case "all":
		log.SetLevel(log.TraceLevel)
		log.Debug("All messages will be logged out")
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	case "info":
		log.SetLevel(log.InfoLevel)
		log.Debug("Info logging is enabled")
	}

	if *withTracing {
		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "loader")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}

	switch *server {
	case "wimpy":
		serviceConfigPath = "workloads/container/wimpy.yaml"
	case "trace":
		serviceConfigPath = "workloads/container/trace_func_go.yaml"
		// serviceConfigPath = "workloads/firecracker/trace_func_go.yaml"
	}
	log.Info("Using service config file: ", serviceConfigPath)
}

func main() {
	gen.InitSeed(*seed)

	switch *mode {
	case "trace":
		invPath := *tracePath + strconv.Itoa(*sampleSize) + "_inv.csv"
		runPath := *tracePath + strconv.Itoa(*sampleSize) + "_run.csv"
		memPath := *tracePath + strconv.Itoa(*sampleSize) + "_mem.csv"

		runTraceMode(invPath, runPath, memPath)
	case "stress":
		runStressMode()
	case "coldstart":
		runColdStartMode()
	default:
		log.Fatal("Invalid mode: ", *mode)
	}
}

func runTraceMode(invPath, runPath, memPath string) {
	/** Trace parsing */
	traces = tc.ParseInvocationTrace(invPath, *duration)
	tc.ParseDurationTrace(&traces, runPath)
	tc.ParseMemoryTrace(&traces, memPath)

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Println("\t" + function.GetName())
	}

	totalNumPhases := 3
	profilingMinutes := *duration/2 + 1 //TODO

	/* Profiling */
	if *withWarmup {
		for funcIdx := 0; funcIdx < len(traces.Functions); funcIdx++ {
			function := traces.Functions[funcIdx]
			traces.Functions[funcIdx].ConcurrencySats =
				tc.ProfileFunctionConcurrencies(function, profilingMinutes)
		}
		traces.WarmupScales = wu.ComputeFunctionsWarmupScales(*cluster, traces.Functions)
	}

	/** Deployment */
	functions := fc.DeployTrace(traces.Functions, serviceConfigPath, traces.WarmupScales)

	/** Warmup (Phase 1 and 2) */
	nextPhaseStart := 0
	if *withWarmup {
		nextPhaseStart = wu.Warmup(*sampleSize, totalNumPhases, *rps, functions, traces)
	}

	/** Measurement (Phase 3) */
	if nextPhaseStart == *duration {
		// gen.DumpOverloadFlag()
		log.Infof("Warmup failed to finish in %d minutes", *duration)
	}
	//* Start from the beginning regardless of the warmup.
	nextPhaseStart = 0
	log.Infof("Phase 3: Generate real workloads as of Minute[%d]", nextPhaseStart)
	defer gen.GenerateTraceLoads(
		*sampleSize,
		totalNumPhases,
		nextPhaseStart,
		true,
		*rps,
		functions,
		traces.InvocationsEachMinute[nextPhaseStart:],
		traces.TotalInvocationsPerMinute[nextPhaseStart:])
}

func runStressMode() {
	functions := []tc.Function{}
	initialScales := []int{}

	for i := 0; i < *totalFunctions; i++ {
		stressFunc := "stress-func-" + strconv.Itoa(i)
		functions = append(functions, tc.Function{
			Name:     stressFunc,
			Endpoint: tc.GetFuncEndpoint(stressFunc),
		})
		initialScales = append(initialScales, 1)
	}

	fc.DeployTrace(functions, serviceConfigPath, initialScales)

	defer gen.GenerateStressLoads(*rpsStart, *rpsEnd, *rpsStep, *rpsSlot, functions)
}

func runColdStartMode() {
	coldStartCountFile := "data/coldstarts/200f_30min.csv"
	coldstartCounts := util.ReadIntArray(coldStartCountFile, ",")
	totalFunctions := 200 - 1
	functions := []tc.Function{}

	// Create a single hot function.
	hotFunction := tc.Function{
		Name:     "hot-func",
		Endpoint: tc.GetFuncEndpoint("hot-func"),
	}
	functions = append(functions, hotFunction)
	// Set the rest functions as cold.
	for i := 0; i < totalFunctions; i++ {
		coldFunc := "cold-func-" + strconv.Itoa(i)
		functions = append(functions, tc.Function{
			Name:     coldFunc,
			Endpoint: tc.GetFuncEndpoint(coldFunc),
		})
	}

	fc.DeployTrace(functions, serviceConfigPath, []int{})

	defer gen.GenerateColdStartLoads(*rpsStart, *rpsStep, hotFunction, coldstartCounts)
}
