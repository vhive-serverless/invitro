package main

import (
	"encoding/json"
	"flag"
	_ "fmt"
	"io/ioutil"
	"os"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"

	fc "github.com/eth-easl/easyloader/cmd/function"
)

func main() {
	funcPath := flag.String("funcPath", "../../config/workloads", "Path to the folder with *.yml files")
	funcJSONFile := flag.String("jsonFile", "../function/functions.json", "Path to the JSON file with functions to deploy")
	// endpointsFile := flag.String("endpointsFile", "endpoints.json", "File with endpoints' metadata")
	deploymentConcurrency := flag.Int("conc", 1, "Number of functions to deploy concurrently (for serving)")
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
