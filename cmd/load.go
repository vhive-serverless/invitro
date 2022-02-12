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
	fc "github.com/eth-easl/loader/pkg/function"
	gen "github.com/eth-easl/loader/pkg/generate"
	tc "github.com/eth-easl/loader/pkg/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	traces            tc.FunctionTraces
	serviceConfigPath = "workloads/trace_func_go.yaml"

	debug       = flag.Bool("dbg", false, "Enable debug logging")
	rps         = flag.Int("rps", -900_000, "Request per second")
	duration    = flag.Int("duration", 3, "Duration of the experiment")
	sampleSize  = flag.Int("sample", 1, "Sample size of the traces")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")

	// withWarmup = flag.Int("withWarmup", -1000, "Duration of the withWarmup")
	withWarmup = flag.Bool("warmup", false, "Enable warmup")
)

func init() {
	/** Logging. */
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)
	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}
	if *withTracing {
		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "loader")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}

	/** Trace parsing. */
	traces = tc.ParseInvocationTrace(
		"data/traces/"+strconv.Itoa(*sampleSize)+"/invocations.csv", *duration)
	tc.ParseDurationTrace(
		&traces, "data/traces/"+strconv.Itoa(*sampleSize)+"/durations.csv")
	tc.ParseMemoryTrace(
		&traces, "data/traces/"+strconv.Itoa(*sampleSize)+"/memory.csv")

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Println("\t" + function.GetName())
	}
}

func main() {
	totalNumPhases := 3
	profilingMinutes := *duration/2 + 1 //TODO

	/* Profiling */
	if *withWarmup {
		for funcIdx := 0; funcIdx < len(traces.Functions); funcIdx++ {
			function := traces.Functions[funcIdx]
			traces.Functions[funcIdx].ConcurrencySats =
				tc.ProfileFunctionConcurrencies(function, profilingMinutes)
		}
		traces.WarmupScales = wu.ComputeFunctionsWarmupScales(traces.Functions)
	}

	/** Deployment */
	functions := fc.Deploy(traces.Functions, serviceConfigPath, traces.WarmupScales)

	//TODO: Extract to warmup.go
	/** Warmup (Phase 1 and 2) */
	nextPhaseStart := 0
	if *withWarmup {
		for phaseIdx := 1; phaseIdx < totalNumPhases; phaseIdx++ {
			//* Set up kn environment
			if phaseIdx == 1 {
				wu.SetKnConfigMap("config/kn_configmap_init_patch.yaml")
			}

			log.Infof("Enter Phase %d as of Minute[%d]", phaseIdx, nextPhaseStart)
			nextPhaseStart = gen.GenerateLoads(
				phaseIdx,
				nextPhaseStart,
				false, //! Non-blocking: directly go into the next phase.
				*rps,
				functions,
				traces.InvocationsEachMinute[nextPhaseStart:],
				traces.TotalInvocationsPerMinute[nextPhaseStart:])

			//* Reset kn environment
			if phaseIdx == 1 {
				wu.SetKnConfigMap("config/kn_configmap_reset_patch.yaml")
				wu.LivePatchKpas("scripts/warmup/livepatch_kpas.sh")
			}
		}
	}

	/** Measurement (Phase 3) */
	log.Infof("Phase 3: Generate real workloads as of Minute[%d]", nextPhaseStart)
	defer gen.GenerateLoads(
		totalNumPhases,
		nextPhaseStart,
		true,
		*rps,
		functions,
		traces.InvocationsEachMinute[nextPhaseStart:],
		traces.TotalInvocationsPerMinute[nextPhaseStart:])
}
