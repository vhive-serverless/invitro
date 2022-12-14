package config

import (
	"encoding/json"
	"os"

	log "github.com/sirupsen/logrus"
)

type LoaderConfiguration struct {
	Seed int64 `json:"Seed"`

	YAMLSelector string `json:"YAMLSelector"`
	EndpointPort int    `json:"EndpointPort"`

	TracePath          string `json:"TracePath"`
	OutputPathPrefix   string `json:"OutputPathPrefix"`
	IATDistribution    string `json:"IATDistribution"`
	ExperimentDuration int    `json:"ExperimentDuration"`
	WarmupDuration     int    `json:"WarmupDuration"`

	IsPartiallyPanic       bool `json:"IsPartiallyPanic"`
	EnableZipkinTracing    bool `json:"EnableZipkinTracing"`
	EnableMetricsScrapping bool `json:"EnableMetricsScrapping"`
	MetricScrapingPeriod   int  `json:"MetricScrapingPeriod"`

	GRPCConnectionTimeoutSeconds int `json:"GRPCConnectionTimeoutSeconds"`
	GRPCFunctionTimeoutSeconds   int `json:"GRPCFunctionTimeoutSeconds"`
}

func ReadConfigurationFile(path string) LoaderConfiguration {
	byteValue, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var config LoaderConfiguration
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}
