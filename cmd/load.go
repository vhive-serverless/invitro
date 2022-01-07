package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	wu "github.com/eth-easl/loader/cmd/warmup"
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
	rps         = flag.Int("rps", 1, "Request per second")
	duration    = flag.Int("duration", 1, "Duration of the experiment")
	sampleSize  = flag.Int("sample", 5, "Sample size of the traces")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
	withWarmup  = flag.Bool("warmup", true, "Enable warmup phase")
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
	/** Deployment */
	log.Info("Using service config file: ", serviceConfigPath)
	functions := fc.Deploy(traces.Functions, serviceConfigPath, *withWarmup)
	realInvocationStart := 0

	/** Warmup */
	if *withWarmup {
		// wu.SetKnGlobal("config/kn_init_global_path.yaml")

		totalNumPhases := 3
		phaseDuration := *duration / totalNumPhases

		phasesCh := wu.PhasePartition(len(traces.InvocationsPerMinute), phaseDuration)

		sem := make(chan bool, 1) //* Enforce sequential execution using semphore.

		for i := 1; i < totalNumPhases; i++ {
			sem <- true

			phase := <-phasesCh
			go func(i int) {
				defer func() { <-sem }()

				log.Infof("Start warmup phase %d in range [%d, %d)", i, phase.Start, phase.End)
				fc.Generate(*rps, functions,
					traces.InvocationsPerMinute[phase.Start:phase.End],
					traces.TotalInvocationsEachMinute[phase.Start:phase.End])
			}(i)
		}

		wu.SetKnGlobal("config/kn_reset_global_path.yaml")
		realInvocationStart = (<-phasesCh).Start
	}

	/** Invocation */
	defer fc.Generate(*rps, functions,
		traces.InvocationsPerMinute[realInvocationStart:],
		traces.TotalInvocationsEachMinute[realInvocationStart:])
}
