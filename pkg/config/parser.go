/*
 * MIT License
 *
 * Copyright (c) 2023 EASL and the vHive community
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package config

import (
	"encoding/json"
	"os"

	log "github.com/sirupsen/logrus"
)

type FailureConfiguration struct {
	FailureEnabled bool `json:"FailureEnabled"`

	FailAt        int    `json:"FailAt"`
	FailComponent string `json:"FailComponent"`
	FailNode      string `json:"FailNode"`
}

type LoaderConfiguration struct {
	Seed int64 `json:"Seed"`

	Platform       string `json:"Platform"`
	InvokeProtocol string `json:"InvokeProtocol"`
	YAMLSelector   string `json:"YAMLSelector"`
	EndpointPort   int    `json:"EndpointPort"`

	DirigentControlPlaneIP   string `json:"DirigentControlPlaneIP"`
	BusyLoopOnSandboxStartup bool   `json:"BusyLoopOnSandboxStartup"`

	AsyncMode             bool   `json:"AsyncMode"`
	AsyncResponseURL      string `json:"AsyncResponseURL"`
	AsyncWaitToCollectMin int    `json:"AsyncWaitToCollectMin"`

	RpsTarget                   float64 `json:"RpsTarget"`
	RpsColdStartRatioPercentage float64 `json:"RpsColdStartRatioPercentage"`
	RpsCooldownSeconds          int     `json:"RpsCooldownSeconds"`
	RpsImage                    string  `json:"RpsImage"`
	RpsRuntimeMs                int     `json:"RpsRuntimeMs"`
	RpsMemoryMB                 int     `json:"RpsMemoryMB"`
	RpsRequestedGpu             int     `json:"RpsRequestedGpu"`
	RpsIterationMultiplier      int     `json:"RpsIterationMultiplier"`
	RpsDataSizeMB               float64 `json:"RpsDataSizeMB"`
	RpsFile                     string  `json:"RpsFile"`

	TracePath          string `json:"TracePath"`
	Granularity        string `json:"Granularity"`
	OutputPathPrefix   string `json:"OutputPathPrefix"`
	IATDistribution    string `json:"IATDistribution"`
	CPULimit           string `json:"CPULimit"`
	ExperimentDuration int    `json:"ExperimentDuration"`
	WarmupDuration     int    `json:"WarmupDuration"`
	PrepullMode        string `json:"PrepullMode"`

	IsPartiallyPanic            bool   `json:"IsPartiallyPanic"`
	EnableZipkinTracing         bool   `json:"EnableZipkinTracing"`
	EnableMetricsScrapping      bool   `json:"EnableMetricsScrapping"`
	MetricScrapingPeriodSeconds int    `json:"MetricScrapingPeriodSeconds"`
	AutoscalingMetric           string `json:"AutoscalingMetric"`

	GRPCConnectionTimeoutSeconds int  `json:"GRPCConnectionTimeoutSeconds"`
	GRPCFunctionTimeoutSeconds   int  `json:"GRPCFunctionTimeoutSeconds"`
	DAGMode                      bool `json:"DAGMode"`
	EnableDAGDataset             bool `json:"EnableDAGDataset"`
	Width                        int  `json:"Width"`
	Depth                        int  `json:"Depth"`
	VSwarm                       bool `json:"VSwarm"`

	// used only for dirigent-dandelion workflows
	WorkflowConfigPath string `json:"WorkflowConfigPath"`
}

type WorkflowFunction struct {
	FunctionName string `json:"FunctionName"`
	FunctionPath string `json:"FunctionPath"`
	NumArgs      int    `json:"NumArgs"`
	NumRets      int    `json:"NumRets"`
}
type CompositionConfig struct {
	Name        string     `json:"Name"`
	InDataPaths [][]string `json:"InDataPaths"`
}
type WorkflowConfig struct {
	Name         string              `json:"Name"`
	Functions    []WorkflowFunction  `json:"Functions"`
	Compositions []CompositionConfig `json:"Compositions"`
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

func ReadFailureConfiguration(path string) *FailureConfiguration {
	byteValue, err := os.ReadFile(path)
	if err != nil {
		log.Warnf("Failure configuration not found at '%s'...", path)
		return &FailureConfiguration{}
	}

	log.Infof("Failure configuration found.")

	var config FailureConfiguration
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatal(err)
	}

	return &config
}

func ReadWorkflowConfig(path string) WorkflowConfig {
	byteValue, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var config WorkflowConfig
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}
