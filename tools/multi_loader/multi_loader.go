package main

import (
	"flag"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/tools/multi_loader/runner"
)

var (
	multiLoaderConfigPath    = flag.String("multiLoaderConfig", "tools/multi_loader/multi_loader_config.json", "Path to multi loader configuration file")
    verbosity     = flag.String("verbosity", "info", "Logging verbosity - choose from [info, debug, trace]")
	iatGeneration = flag.Bool("iatGeneration", false, "Generate iats only and skip invocations")
	generated     = flag.Bool("generated", false, "If iats were already generated")
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
	// Create multi loader driver
	multiLoaderDriver, err := runner.NewMultiLoaderRunner(*multiLoaderConfigPath, *verbosity, *iatGeneration, *generated)
	if err != nil {
		log.Fatalf("Failed to create multi loader driver: %v", err)
	}
	// Dry run
	multiLoaderDriver.RunDryRun()

	// Check if dry run was successful
	if !multiLoaderDriver.DryRunSuccess {
		log.Fatal("Dry run failed. Exiting...")
	}

	// Actual run
	log.Info("Running experiments")
	multiLoaderDriver.RunActual()

	// Finish
	log.Info("All experiments completed")
}
