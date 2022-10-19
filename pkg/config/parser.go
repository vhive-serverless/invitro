package config

import (
	"encoding/json"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
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

	GRPCConnectionTimeoutSeconds int `json:"GRPCConnectionTimeoutSeconds"`
	GRPCFunctionTimeoutSeconds   int `json:"GRPCFunctionTimeoutSeconds"`
}

func ReadConfigurationFile(path string) LoaderConfiguration {
	jsonFile, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}

	byteValue, err := ioutil.ReadAll(jsonFile)
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
