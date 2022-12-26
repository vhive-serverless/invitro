package main

import (
	"encoding/csv"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"os"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

var loaderCfg = loaderConfig{
	configPath: "loaderConfig_1.json",
	loaderConfiguration: LoaderConfiguration{
		Seed:         42,
		YAMLSelector: "container",
		EndpointPort: 80,

		TracePath:          "data/traces",
		OutputPathPrefix:   "data/out/experiment",
		IATDistribution:    "exponential",
		ExperimentDuration: 2,
		WarmupDuration:     10,

		IsPartiallyPanic:       false,
		EnableZipkinTracing:    false,
		EnableMetricsScrapping: false,

		GRPCConnectionTimeoutSeconds: 60,
		GRPCFunctionTimeoutSeconds:   900,
	},
	functions: 1,
}

var testDriver = Driver{
	Username:               "",
	ExperimentName:         "example",
	LoaderAddresses:        []string{"localhost"},
	clients:                nil,
	BeginningFuncNum:       1,
	StepSizeFunc:           1,
	MaxFuncNum:             1,
	ExperimentDuration:     2,
	WarmupDuration:         10,
	WorkerNodeNum:          1,
	LocalTracePath:         "trace/example",
	LoaderTracePath:        "loader/data/traces",
	OutputDir:              "measurements",
	YAMLSelector:           "container",
	IATDistribution:        "exponential",
	LoaderOutputPath:       "data/out/experiment",
	PartiallyPanic:         false,
	EnableZipkinTracing:    false,
	EnableMetricsScrapping: false,
	SeparateIATGeneration:  false,
	loaderConfig:           loaderCfg,
}

var testClient = connectToLoader([]string{"localhost"}, "")[0]
var testWg sync.WaitGroup

func TestNewDriver(t *testing.T) {
	tests := []struct {
		testName   string
		configFile string
		expected   []Driver
	}{
		{
			testName:   "basic",
			configFile: "testDriverConfig.json",
			expected:   []Driver{testDriver},
		},
	}

	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if got := NewDriver(tt.configFile); !reflect.DeepEqual(*got, tt.expected) {
				t.Errorf("NewDriver() = %v, want %v", *got, tt.expected)
			}
		})
	}
}

func TestDriver_createLoaderConfig(t *testing.T) {
	tests := []struct {
		testName       string
		driver         Driver
		functionNumber int
		expected       loaderConfig
	}{
		{
			testName:       "basic",
			driver:         testDriver,
			functionNumber: 1,
			expected:       loaderCfg,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if got := testDriver.createLoaderConfig(tt.functionNumber); !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("createLoaderConfig() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func Test_connectToLoader(t *testing.T) {
	tests := []struct {
		testName        string
		loaderAddresses []string
		username        string
		expected        int
	}{
		{
			testName:        "basic",
			loaderAddresses: []string{"localhost"},
			username:        "",
			expected:        1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			if got := connectToLoader(tt.loaderAddresses, tt.username); !reflect.DeepEqual(len(got), tt.expected) {
				t.Errorf("connectToLoader() = %v, want %v", len(got), tt.expected)
			}
		})
	}
}

func TestDriver_transferFilesToLoader(t *testing.T) {
	tests := []struct {
		testName string
		driver   Driver
		client   *simplessh.Client
		wg       *sync.WaitGroup
	}{
		{
			testName: "basic",
			driver:   testDriver,
			client:   testClient,
			wg:       &testWg,
		},
	}
	for _, tt := range tests {

		t.Run(tt.testName, func(t *testing.T) {
			testDriver.clients = connectToLoader(testDriver.LoaderAddresses, testDriver.Username)
			tt.wg.Add(1)
			testDriver.transferFilesToLoader(tt.client, tt.wg, 0)
			tt.wg.Wait()
			homedir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("Failed to get home directory: %s", err)
			}
			_, err = os.Stat(homedir + "/loader/cmd/" + testDriver.loaderConfig.configPath)
			if err != nil {
				t.Errorf("Loader config file %s does not exist: %s", "loader/cmd/"+testDriver.loaderConfig.configPath, err)
			}
			traceFiles := [3]string{"invocations.csv", "memory.csv", "durations.csv"}
			for _, i := range traceFiles {
				_, err := os.Stat(homedir + "/" + testDriver.LoaderTracePath + i)
				if err != nil {
					t.Errorf("Trace file %s does not exist: %s", homedir+"/"+testDriver.LoaderTracePath+i, err)
				}
			}
		})

	}
}

func TestDriver_partitionTraceFiles(t *testing.T) {
	tests := []struct {
		testName string
		driver   Driver
		client   *simplessh.Client
	}{
		{
			testName: "basic",
			driver:   testDriver,
			client:   testClient,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			testDriver.clients = connectToLoader(testDriver.LoaderAddresses, testDriver.Username)
			testDriver.partitionTraceFiles()
			traceFiles := [3]string{"1_inv_part_0.csv", "1_mem_part_0.csv", "1_run_part_0.csv"}
			for _, i := range traceFiles {
				path := testDriver.LocalTracePath + "/" + i
				_, err := os.Stat(path)
				if err != nil {
					t.Errorf("Trace file %s does not exist: %s", path, err)
				}
				f, err := os.Open(path)
				reader := csv.NewReader(f)
				records, err := reader.ReadAll()
				if err != nil {
					t.Errorf("Invalid trace structure for file %s with error: %s", path, err)
				}
				if len(records) < 2 {
					t.Errorf("Trace file %s is empty", path)
				}
			}
		})
	}
}

func TestDriver_prepareLoader(t *testing.T) {
	tests := []struct {
		testName string
		driver   Driver
		client   *simplessh.Client
		wg       *sync.WaitGroup
	}{
		{
			testName: "basic",
			driver:   testDriver,
			client:   testClient,
			wg:       &testWg,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			testDriver.clients = connectToLoader(testDriver.LoaderAddresses, testDriver.Username)
			tt.wg.Add(1)
			testDriver.prepareLoader(tt.client, tt.wg)
			tt.wg.Wait()
			homedir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("Failed to get home directory: %s", err)
			}
			_, err = os.Stat(homedir + "/loader/iat0.json")
			if err != nil {
				t.Errorf("iat file %s does not exist: %s", "/loader/iat0.json", err)
			}
		})
	}
}

func TestDriver_collectStats(t *testing.T) {
	tests := []struct {
		testName string
		driver   Driver
		client   *simplessh.Client
	}{
		{
			testName: "basic",
			driver:   testDriver,
			client:   testClient,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			testDriver.clients = connectToLoader(testDriver.LoaderAddresses, testDriver.Username)
			testDriver.collectStats()
			path := testDriver.OutputDir + "/" + testDriver.ExperimentName + "/experiment_duration_"
			path = path + strconv.Itoa(testDriver.ExperimentDuration+testDriver.WarmupDuration+1) + "_"
			path = path + strconv.Itoa(testDriver.loaderConfig.functions) + "functions_part_0.csv"
			_, err := os.Stat(path)
			if err != nil {
				t.Errorf("Result file %s does not exist: %s", path, err)
			}
			f, err := os.Open(path)
			reader := csv.NewReader(f)
			records, err := reader.ReadAll()
			if err != nil {
				t.Errorf("Invalid structure for result file %s with error: %s", path, err)
			}
			if len(records) < 2 {
				t.Errorf("Result file %s is empty", path)
			}
		})
	}
}

func TestDriver_aggregateStats(t *testing.T) {
	tests := []struct {
		testName string
		driver   Driver
		client   *simplessh.Client
	}{
		{
			testName: "basic",
			driver:   testDriver,
			client:   testClient,
		},
	}
	for _, tt := range tests {
		t.Run(tt.testName, func(t *testing.T) {
			testDriver.aggregateStats()
			path := testDriver.OutputDir + "/" + testDriver.ExperimentName + "/experiment_duration_"
			path = path + strconv.Itoa(testDriver.ExperimentDuration+testDriver.WarmupDuration+1) + "_"
			path = path + strconv.Itoa(testDriver.loaderConfig.functions) + "functions_aggregated.csv"
			_, err := os.Stat(path)
			if err != nil {
				t.Errorf("Aggregated result file %s does not exist: %s", path, err)
			}
			f, err := os.Open(path)
			reader := csv.NewReader(f)
			records, err := reader.ReadAll()
			if err != nil {
				t.Errorf("Invalid structure for aggregated result file %s with error: %s", path, err)
			}
			pathPart := testDriver.OutputDir + "/" + testDriver.ExperimentName + "/experiment_duration_"
			pathPart = pathPart + strconv.Itoa(testDriver.ExperimentDuration+testDriver.WarmupDuration+1) + "_"
			pathPart = pathPart + strconv.Itoa(testDriver.loaderConfig.functions) + "functions_part_0.csv"
			_, err = os.Stat(pathPart)
			if err != nil {
				t.Errorf("Partial result file %s does not exist: %s", pathPart, err)
			}
			filePart, err := os.Open(pathPart)
			readerPart := csv.NewReader(filePart)
			recordsPart, err := readerPart.ReadAll()
			if err != nil {
				t.Errorf("Invalid structure for result file %s with error: %s", pathPart, err)
			}
			if len(records) != len(recordsPart) {
				t.Errorf("Aggregated result file %s has a different number of rows than the sum of the parts"+
					" Part file has %d rows, aggregated result file has %d rows.",
					path, len(recordsPart), len(records))
			}
		})
	}
}
