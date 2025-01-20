package main

import (
	"flag"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/tools/multi_loader/runner"
)

var (
	multiLoaderConfigPath = flag.String("multiLoaderConfigPath", "tools/multi_loader/multi_loader_config.json", "Path to multi loader configuration file")
	verbosity             = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	failFast              = flag.Bool("failFast", false, "Determines whether a study should immediately skip to the next study upon failure")
)

func init() {
	flag.Parse()
	initLogger()
}

func initLogger() {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: time.StampMilli,
		FullTimestamp:   true,
	})
	log.SetOutput(os.Stdout)

	switch *verbosity {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "trace":
		log.SetLevel(log.TraceLevel)
	default:
		log.SetLevel(log.InfoLevel)
	}
}

func main() {
	log.Info("Starting multiloader")
	// Create multi loader runner
	multiLoaderRunner, err := runner.NewMultiLoaderRunner(*multiLoaderConfigPath, *verbosity, *failFast)
	if err != nil {
		log.Fatalf("Failed to create multi loader driver: %v", err)
	}
	// Dry run
	multiLoaderRunner.RunDryRun()

	// Check if dry run was successful
	if !multiLoaderRunner.DryRunSuccess {
		log.Fatal("Dry run failed. Exiting...")
	}

	// Actual run
	multiLoaderRunner.RunActual()

	// Finish
	log.Info("All studies completed")
}
