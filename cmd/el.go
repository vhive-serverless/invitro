package main

import (
	"encoding/json"
	"flag"
	_ "fmt"
	"io/ioutil"
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
	funcPath := flag.String("funcPath", "workloads", "Path to the folder with *.yml files")
	funcJSONFile := flag.String("jsonFile", "config/functions.json", "Path to the JSON file with functions to deploy")
	deploymentConcurrency := flag.Int("conc", 1, "Number of functions to deploy concurrently (for serving)")

	traces := tc.ParseInvocationTrace("data/invocations_10.csv", 1)
	log.Info(traces)
	tc.ParseDurationTrace(&traces, "data/durations_10.csv")
	tc.ParseMemoryTrace(&traces, "data/memory_10.csv")

	/* Deployment */
	log.Info("Function files are taken from ", *funcPath)
	funcSlice := getFuncSlice(*funcJSONFile)
	endpoints := fc.Deploy(*funcPath, funcSlice, *deploymentConcurrency)

	log.Info("Function endpoints: ", endpoints)

	/* Invokation */
	fc.Invoke(endpoints)
}

func getFuncSlice(file string) []fc.FunctionType {
	log.Info("Opening JSON file with functions: ", file)
	byteValue, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	var functions fc.Functions
	if err := json.Unmarshal(byteValue, &functions); err != nil {
		log.Fatal(err)
	}
	return functions.Functions
}
