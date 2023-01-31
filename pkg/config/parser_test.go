package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestConfigParser(t *testing.T) {
	wd, _ := os.Getwd()

	var pathToConfigFile = wd
	if strings.HasSuffix(wd, "pkg/config") {
		pathToConfigFile += "/../../"
	}
	pathToConfigFile += "cmd/config.json"

	fmt.Println(pathToConfigFile)

	config := ReadConfigurationFile(pathToConfigFile)

	if config.Seed != 42 ||
		config.YAMLSelector != "container" ||
		config.EndpointPort != 80 ||
		!strings.HasPrefix(config.TracePath, "data/traces") ||
		!strings.HasPrefix(config.OutputPathPrefix, "data/out/experiment") ||
		config.Granularity != "minute" ||
		config.IATDistribution != "equidistant" ||
		config.ExperimentDuration != 45 ||
		config.WarmupDuration != 0 ||
		config.IsPartiallyPanic != false ||
		config.EnableZipkinTracing != false ||
		config.EnableMetricsScrapping != false ||
		config.MetricScrapingPeriodSeconds != 15 ||
		config.GRPCConnectionTimeoutSeconds != 60 ||
		config.GRPCFunctionTimeoutSeconds != 900 {

		t.Error("Unexpected configuration read.")
	}
}
