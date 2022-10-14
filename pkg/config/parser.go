package config

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
)

type LoaderConfiguration struct {
	Seed int64 `json:"Seed"`

	YAMLSelector string `json:"YAMLSelector"`
	EndpointPort int    `json:"EndpointPort"`

	TracePath          string `json:"TracePath"`
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
		logrus.Fatal("Error opening configuration file.")
	}

	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		logrus.Fatal("Error reading configuration file.")
	}

	var config LoaderConfiguration
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		logrus.Fatal("Error unmarshalling configuration file.")
	}

	return config
}
