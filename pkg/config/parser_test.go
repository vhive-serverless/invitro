package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigParser(t *testing.T) {
	var pathToConfigFile = ""
	wd, _ := os.Getwd()

	if strings.HasSuffix(wd, "pkg/config") {
		pathToConfigFile = "../../"
	}
	pathToConfigFile += "cmd/config.json"
	
	config := ReadConfigurationFile(pathToConfigFile)

	if config.Seed != 42 ||
		config.YAMLSelector != "container" ||
		config.EndpointPort != 80 ||
		config.TracePath != "data/traces" ||
		config.OutputPathPrefix != "data/out/experiment" ||
		config.IATDistribution != "exponential" ||
		config.ExperimentDuration != 1 ||
		config.WarmupDuration != 0 ||
		config.IsPartiallyPanic != false ||
		config.EnableZipkinTracing != false ||
		config.EnableMetricsScrapping != false ||
		config.GRPCConnectionTimeoutSeconds != 60 ||
		config.GRPCFunctionTimeoutSeconds != 900 {

		t.Error("Unexpected configuration read.")
	}
}
