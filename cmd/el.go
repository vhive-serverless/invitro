package main

import (
	// "encoding/json"
	"flag"
	"fmt"
	"time"

	// "io/ioutil"
	"os"

	log "github.com/sirupsen/logrus"

	tracer "github.com/ease-lab/vhive/utils/tracing/go"
	fc "github.com/eth-easl/easyloader/internal/function"
	tc "github.com/eth-easl/easyloader/internal/trace"
)

const (
	zipkinAddr = "http://localhost:9411/api/v2/spans"
)

var (
	debug       = flag.Bool("dbg", false, "Enable debug logging")
	rps         = flag.Int("rps", 1, "Request per second")
	duration    = flag.Int("duration", 1, "Duration of the experiment")
	withTracing = flag.Bool("trace", false, "Enable tracing in the client")
)

func init() {

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
		shutdown, err := tracer.InitBasicTracer(zipkinAddr, "invoker")
		if err != nil {
			log.Print(err)
		}
		defer shutdown()
	}
}

func main() {
	// serviceConfigPath := "workloads/producer.yaml"
	serviceConfigPath := "workloads/trace_func.yaml"
	traces := tc.ParseInvocationTrace("data/traces/invocations_10.csv", *duration)
	tc.ParseDurationTrace(&traces, "data/traces/durations_10.csv")
	tc.ParseMemoryTrace(&traces, "data/traces/memory_10.csv")

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Println("\t" + function.GetName())
	}

	/** Deployment */
	log.Info("Using service config file: ", serviceConfigPath)
	functions := fc.Deploy(traces.Functions, serviceConfigPath, 1) // TODO: Fixed number of functions per pod.

	/** Invokation */
	defer fc.Invoke(*rps, functions, traces.InvocationsPerMin, traces.TotalInvocationsEachMin)
}
