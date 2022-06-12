package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	opts "github.com/eth-easl/loader/cmd/options"
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

	mode      = flag.String("mode", "trace", "Choose a mode from [trace, stress, burst, coldstart]")
	server    = flag.String("server", "trace", "Choose a function server from [wimpy, trace]")
	tracePath = flag.String("tracePath", "data/traces/", "Path to trace")

	cluster    = flag.Int("cluster", 1, "Size of the cluster measured by #workers")
	duration   = flag.Int("duration", 3, "Duration of the experiment in minutes")
	sampleSize = flag.Int("sample", 10, "Sample size of the traces")

	rpsStart       = flag.Int("start", 0, "Starting RPS value")
	rpsEnd         = flag.Int("end", -900_000, "Final RPS value")
	rpsSlot        = flag.Int("slot", 60, "Time slot in seconds for each RPS in the `stress` mode")
	rpsStep        = flag.Int("step", 1, "Step size for increasing RPS in the `stress` mode")
	totalFunctions = flag.Int("totalFunctions", 1, "Total number of functions used in the `stress` mode")

	burstTarget = flag.Int("burst", 10, "The target volumn of burst")

	funcDuration = flag.Int("funcDuration", 1000, "Function execution duration in ms (under `stress` mode)")
	funcMemory   = flag.Int("funcMemory", 170, "Function memeory in MiB(under `stress` mode)")

	seed  = flag.Int64("seed", 42, "Random seed for the generator")
	print = flag.String("print", "all", "Choose a mode from [all, debug, info]")

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
	case "burst":
		runBurstMode()
	case "coldstart":
		runColdStartMode()
	default:
		log.Fatal("Invalid mode: ", *mode)
	}
	fc.DestroyGrpcPool()
}

func runTraceMode(invPath, runPath, memPath string) {
	/** Trace parsing */
	if *duration < 1 {
		log.Fatal("Trace duration should be longer than 0 minutes.")
	}
	traces = tc.ParseInvocationTrace(invPath, util.MinOf(1440, *duration*gen.TRACE_WARMUP_DURATION))
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
		traces.WarmupScales = opts.ComputeFunctionsWarmupScales(*cluster, traces.Functions)
	}

	/** Deployment */
	functions := fc.DeployFunctions(traces.Functions, serviceConfigPath, traces.WarmupScales)

	/** Warmup (Phase 1 and 2) */
	nextPhaseStart := 0
	if *withWarmup {
		nextPhaseStart = opts.Warmup(*sampleSize, totalNumPhases, functions, traces)
	}

	/** Measurement (Phase 3) */
	if nextPhaseStart == *duration {
		// gen.DumpOverloadFlag()
		log.Warn("Warmup failed to finish in %d minutes", *duration)
	}

	log.Infof("Phase 3: Generate real workloads as of Minute[%d]", nextPhaseStart)
	defer gen.GenerateTraceLoads(
		*sampleSize,
		totalNumPhases,
		nextPhaseStart,
		true,
		functions,
		traces.InvocationsEachMinute[nextPhaseStart:nextPhaseStart+*duration],
		traces.TotalInvocationsPerMinute[nextPhaseStart:nextPhaseStart+*duration])
}

func runStressMode() {
	functions := []tc.Function{}
	initialScales := []int{}

	for i := 0; i < *totalFunctions; i++ {
		stressFunc := "stress-func-" + strconv.Itoa(i)
		functions = append(functions, tc.Function{
			Name:     stressFunc,
			Endpoint: tc.GetFuncEndpoint(stressFunc),
			RuntimeStats: tc.FunctionRuntimeStats{
				Average: *funcDuration,
				Maximum: 0,
			},
			MemoryStats: tc.FunctionMemoryStats{
				Average:       *funcMemory,
				Percentile100: 0,
			},
		})
		initialScales = append(initialScales, 1)
	}

	fc.DeployFunctions(functions, serviceConfigPath, initialScales)

	defer gen.GenerateStressLoads(*rpsStart, *rpsEnd, *rpsStep, *rpsSlot, functions)
}

func runBurstMode() {
	var functions []tc.Function
	functionsTable := make(map[string]tc.Function)
	initialScales := []int{1, 1, 0}

	for _, f := range []string{"steady", "bursty", "victim"} {
		functionsTable[f] = tc.Function{
			Name:     f + "-func",
			Endpoint: tc.GetFuncEndpoint(f + "-func"),
			RuntimeStats: tc.FunctionRuntimeStats{
				Average: *funcDuration,
				Maximum: 0,
			},
			MemoryStats: tc.FunctionMemoryStats{
				Average:       *funcMemory,
				Percentile100: 0,
			},
		}
		functions = append(functions, functionsTable[f])
	}

	fc.DeployFunctions(functions, serviceConfigPath, initialScales)

	defer gen.GenerateBurstLoads(*rpsEnd, *burstTarget, *duration, functionsTable)
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

	fc.DeployFunctions(functions, serviceConfigPath, []int{})

	defer gen.GenerateColdStartLoads(*rpsStart, *rpsStep, hotFunction, coldstartCounts)
}
