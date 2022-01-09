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
	fc "github.com/eth-easl/loader/internal/function"
	tc "github.com/eth-easl/loader/internal/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	traces            tc.FunctionTraces
	serviceConfigPath = "workloads/trace_func_go.yaml"

	debug       = flag.Bool("dbg", false, "Enable debug logging")
	rps         = flag.Int("rps", -900_000, "Request per second")
	duration    = flag.Int("duration", 30, "Duration of the experiment")
	sampleSize  = flag.Int("sample", 1, "Sample size of the traces")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")

	withWarmup     = flag.Bool("warmup", true, "Enable warmup phase")
	warmupDuration = flag.Int("warmup-time", 20, "Duration of the warmup")
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
	var measurementStart int

	/* Profiling */
	if *withWarmup {
		for funcIdx := 0; funcIdx < len(traces.Functions); funcIdx++ {
			function := traces.Functions[funcIdx]
			traces.Functions[funcIdx].ConcurrencySats =
				tc.ProfileFunctionConcurrencies(function, *warmupDuration)
		}
		//* `WarmupScales` are initialised to 0's by default.
		traces.WarmupScales = wu.ComputeFunctionsWarmupScales(traces.Functions)
	}

	/** Deployment */
	functions := fc.Deploy(traces.Functions, serviceConfigPath, traces.WarmupScales)

	/** Warmup (Phase 1 and 2) */
	if *withWarmup {
		//* Enforce sequential execution using semphore.
		sem := make(chan bool, 1)

		//* Partition warmup duration equally over phase 1 and 2.
		phaseDuration := *warmupDuration / 2
		phasesCh := wu.GetPhasePartitions(*warmupDuration, phaseDuration)

		var phase wu.IdxRange
		for phaseIdx := 1; phaseIdx < totalNumPhases; phaseIdx++ {
			sem <- true

			go func(phaseIdx int) {
				defer func() { <-sem }()
				//* Set up kn environment
				if phaseIdx == 1 {
					wu.SetKnConfigMap("config/kn_configmap_init_patch.yaml")
				}

				phase = <-phasesCh
				log.Infof("Phase %d: Warmup within [%d, %d)", phaseIdx, phase.Start, phase.End)
				fc.Generate(
					phaseIdx,
					phase.Start,
					false, //! Non-blocking: directly go into the next phase.
					*rps,
					functions,
					traces.InvocationsEachMinute[phase.Start:phase.End],
					traces.TotalInvocationsPerMinute[phase.Start:phase.End])

				//* Reset kn environment
				if phaseIdx == 1 {
					wu.SetKnConfigMap("config/kn_configmap_reset_patch.yaml")
					wu.LivePatchKpas("scripts/warmup/livepatch_kpas.sh")
				}
			}(phaseIdx)
		}

		//* Block until all slots are refilled (after they have all been consumed).
		for i := 0; i < cap(sem); i++ {
			sem <- true
		}

		measurementStart = phase.End
	}

	log.Info("Phase 3: Generate real workloads as of minute index: ", measurementStart)
	/** Measurement (Phase 3) */
	defer fc.Generate(
		3,
		measurementStart,
		true,
		*rps,
		functions,
		traces.InvocationsEachMinute[measurementStart:],
		traces.TotalInvocationsPerMinute[measurementStart:])
}
