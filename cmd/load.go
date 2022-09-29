package main

import (
	"flag"
	"fmt"
	"github.com/eth-easl/loader/pkg/common"
	"github.com/eth-easl/loader/pkg/driver"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	opts "github.com/eth-easl/loader/cmd/options"
	fc "github.com/eth-easl/loader/pkg/function"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	traces  common.FunctionTraces
	iatType common.IatDistribution

	serviceConfigPath = ""

	mode      = flag.String("mode", "trace", "Choose a mode from [trace, stress, burst, coldstart]")
	server    = flag.String("server", "trace", "Choose a function server from [wimpy, trace]")
	tracePath = flag.String("tracePath", "data/traces/", "Path to trace")

	cluster    = flag.Int("cluster", 1, "Size of the cluster measured by #workers")
	duration   = flag.Int("duration", 3, "Duration of the experiment in minutes")
	sampleSize = flag.Int("sample", 10, "Sample size of the traces")

	rpsStart       = flag.Int("start", 0, "Starting RPS value")
	rpsEnd         = flag.Int("end", -900_000, "Final RPS value")
	rpsSlot        = flag.Int("slot", 1, "Time slot in minutes for each RPS in the `stress` mode")
	rpsStep        = flag.Int("step", 1, "Step size for increasing RPS in the `stress` mode")
	totalFunctions = flag.Int("totalFunctions", 1, "Total number of functions used in the `stress` mode")

	burstTarget = flag.Int("burst", 10, "The target volume of burst")

	funcDuration = flag.Int("funcDuration", 1000, "Function execution duration in ms (under `stress` mode)")
	funcMemory   = flag.Int("funcMemory", 170, "Function memory in MiB(under `stress` mode)")

	iatDistribution = flag.String("iatDistribution", "exponential", "Choose a distribution from [exponential, uniform, equidistant]")

	seed  = flag.Int64("seed", 42, "Random seed for the generator")
	print = flag.String("print", "all", "Choose a mode from [all, debug, info]")

	isPartiallyPanic = flag.Bool("partiallyPanic", false, "Enable partially panic")
	withWarmup       = flag.Bool("warmup", false, "Enable warmup")
	withTracing      = flag.Bool("trace", false, "Enable tracing in the client")
)

// TODO: this file has not been yet reviewed

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

	switch *server {
	case "wimpy":
		serviceConfigPath = "workloads/container/wimpy.yaml"
	case "trace":
		serviceConfigPath = "workloads/container/trace_func_go.yaml"
	case "trace_firecracker":
		serviceConfigPath = "workloads/firecracker/trace_func_go.yaml"
	}

	log.Info("Using service config file: ", serviceConfigPath)
}

func main() {
	if *withTracing {
		// TODO: how not to exlude Zipkin spans here? - file a feature request
		log.Info("Loader Zipkin tracing has been enabled. This will exclude Istio spans from the Zipkin traces.")

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

	switch *mode {
	case "trace":
		invPath := *tracePath + strconv.Itoa(*sampleSize) + "_inv.csv"
		runPath := *tracePath + strconv.Itoa(*sampleSize) + "_run.csv"
		memPath := *tracePath + strconv.Itoa(*sampleSize) + "_mem.csv"

		runTraceMode(invPath, runPath, memPath)
	case "stress":
		//runStressMode()
	case "burst":
		//runBurstMode()
	case "coldstart":
		//runColdStartMode()
	default:
		log.Fatal("Invalid mode: ", *mode)
	}

	// fc.DestroyGrpcPool()
}

func runTraceMode(invPath, runPath, memPath string) {
	/** Trace parsing */
	if *duration < 1 {
		log.Fatal("Trace duration should be longer than 0 minutes.")
	}

	amendedDuration := *duration
	if *withWarmup {
		amendedDuration += common.WARMUP_DURATION_MINUTES * 2
	}

	traces = tc.ParseInvocationTrace(invPath, common.MinOf(1440, amendedDuration))
	tc.ParseDurationTrace(&traces, runPath)
	tc.ParseMemoryTrace(&traces, memPath)

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Println("\t" + function.GetName())
	}

	totalNumPhases := 2

	/* Profiling */
	if *withWarmup {
		for funcIdx := 0; funcIdx < len(traces.Functions); funcIdx++ {
			function := traces.Functions[funcIdx]
			traces.Functions[funcIdx] = tc.ProfileFunction(function, common.PROFILING_DURATION_MINUTES)
		}
		traces.WarmupScales = opts.ComputeFunctionWarmupScales(*cluster, traces.Functions)
	}

	/** Deployment */
	functions := fc.DeployFunctions(traces.Functions, serviceConfigPath, traces.WarmupScales, *isPartiallyPanic)

	/** Warmup (Phase 1) */
	nextPhaseStart := 0
	if *withWarmup {
		nextPhaseStart = opts.Warmup(*sampleSize, totalNumPhases, functions, traces, iatType, *withTracing, *seed)
	}

	/** Measurement (Phase 2) */
	if nextPhaseStart == *duration {
		// gen.DumpOverloadFlag()
		log.Warnf("Warmup failed to finish in %d minutes", *duration)
	}

	log.Infof("Phase 2 - Generate real workloads")

	traceLoadParams := &driver.DriverConfiguration{
		Functions:                     functions,
		TotalNumInvocationsEachMinute: traces.TotalInvocationsPerMinute[nextPhaseStart : nextPhaseStart+*duration],
		IATDistribution:               iatType,
		WithTracing:                   *withTracing,
		Seed:                          *seed,
	}
	driver.NewDriver(traceLoadParams).RunExperiment()
}

/*func runStressMode() {
	var functions []common.Function
	var initialScales []int

	for i := 0; i < *totalFunctions; i++ {
		stressFunc := "stress-func-" + strconv.Itoa(i)
		functions = append(functions, common.Function{
			Name:     stressFunc,
			Endpoint: tc.GetFuncEndpoint(stressFunc),
			//! Set best-effort for RPS sweeping.
			CpuRequestMilli:  0,
			MemoryRequestMiB: 0,
			RuntimeStats: common.FunctionRuntimeStats{
				Count:   -1,
				Average: *funcDuration,
			},
			MemoryStats: common.FunctionMemoryStats{
				Count:   -1,
				Average: *funcMemory,
			},
		})
		initialScales = append(initialScales, 1)
	}

	fc.DeployFunctions(functions, serviceConfigPath, initialScales, *isPartiallyPanic)

	//driver.GenerateStressLoads(*rpsStart, *rpsEnd, *rpsStep, *rpsSlot, functions, iatType, *withTracing, *seed)
}*/

/*func runBurstMode() {
	var functions []common.Function
	functionTable := make(map[string]common.Function)
	initialScales := []int{1, 1, 0}

	for _, f := range []string{"steady", "bursty", "victim"} {
		functionTable[f] = common.Function{
			Name:     f + "-func",
			Endpoint: tc.GetFuncEndpoint(f + "-func"),
			RuntimeStats: common.FunctionRuntimeStats{
				Average: *funcDuration,
				Maximum: 0,
			},
			MemoryStats: common.FunctionMemoryStats{
				Average:       *funcMemory,
				Percentile100: 0,
			},
		}
		functions = append(functions, functionTable[f])
	}

	fc.DeployFunctions(functions, serviceConfigPath, initialScales, *isPartiallyPanic)

	//driver.GenerateBurstLoads(*rpsEnd, *burstTarget, *duration, functionTable, iatType, *withTracing, *seed)
}*/

/*func runColdStartMode() {
	//coldStartCountFile := "data/coldstarts/200f_30min.csv"
	//coldstartCounts := util.ReadIntArray(coldStartCountFile, ",")
	totalFunctions := 200 - 1
	var functions []common.Function

	// Create a single hot function.
	hotFunction := common.Function{
		Name:     "hot-func",
		Endpoint: tc.GetFuncEndpoint("hot-func"),
	}
	functions = append(functions, hotFunction)
	// Set the rest functions as cold.
	for i := 0; i < totalFunctions; i++ {
		coldFunc := "cold-func-" + strconv.Itoa(i)
		functions = append(functions, common.Function{
			Name:     coldFunc,
			Endpoint: tc.GetFuncEndpoint(coldFunc),
		})
	}

	fc.DeployFunctions(functions, serviceConfigPath, []int{}, *isPartiallyPanic)

	//defer driver.GenerateColdStartLoads(*rpsStart, *rpsStep, hotFunction, coldstartCounts, iatType, *withTracing, *seed)
}*/
