package main

import (
	// "encoding/json"
	"flag"
	"fmt"

	// "io/ioutil"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/easyloader/internal/function"
	tc "github.com/eth-easl/easyloader/internal/trace"
)

func init() {
	debug := flag.Bool("dbg", false, "Enable debug logging")
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)
	if *debug {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug logging is enabled")
	} else {
		log.SetLevel(log.InfoLevel)
	}
}

func main() {
	// funcPath := flag.String("funcPath", "workloads", "Path to the folder with *.yml files")
	// funcJSONFile := flag.String("jsonFile", "config/functions.json", "Path to the JSON file with functions to deploy")
	// deploymentConcurrency := flag.Int("conc", 1, "Number of functions to deploy concurrently (for serving)")
	serviceConfigPath := "workloads/timed.yaml"

	traces := tc.ParseInvocationTrace("data/invocations_10.csv", 1)
	tc.ParseDurationTrace(&traces, "data/durations_10.csv")
	tc.ParseMemoryTrace(&traces, "data/memory_10.csv")

	log.Info("Traces contain the following: ", len(traces.Functions), " functions")
	for _, function := range traces.Functions {
		fmt.Println("\t" + function.GetUrl())
	}

	/* Deployment */
	log.Info("Using service config file: ", serviceConfigPath)
	deployedEndpoints := fc.Deploy(traces.Functions, serviceConfigPath, 1) // TODO: Fixed number of functions per pod.

	/* Invokation */
	fc.Invoke(deployedEndpoints)
}
