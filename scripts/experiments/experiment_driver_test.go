package main

import (
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"os"
	"reflect"
	"sync"
	"testing"
)

var loaderCfg = loaderConfig{
	configPath: "loaderConfig_1.json",
	loaderConfiguration: LoaderConfiguration{
		Seed:         42,
		YAMLSelector: "container",
		EndpointPort: 80,

		TracePath:          "data/traces/example/",
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
	Username:           "",
	ExperimentName:     "example",
	LoaderAddresses:    []string{"localhost"},
	clients:            nil,
	BeginningFuncNum:   1,
	StepSizeFunc:       1,
	MaxFuncNum:         1,
	ExperimentDuration: 2,
	WorkerNodeNum:      1,
	LocalTracePath:     "trace/example",
	OutputDir:          "measurements/example",
	loaderConfig:       loaderCfg,
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
			tt.wg.Add(1)
			testDriver.transferFilesToLoader(tt.client, tt.wg)
			tt.wg.Wait()
			homedir, err := os.UserHomeDir()
			if err != nil {
				log.Fatalf("Failed to get home directory: %s", err)
			}
			_, err = os.Stat(homedir + "/loader/cmd/" + testDriver.loaderConfig.configPath)
			if err != nil {
				t.Errorf("Loader config file %s does not exist: %s", "loader/cmd/"+testDriver.loaderConfig.configPath, err)
			}
			traceFiles := [3]string{"1_inv.csv", "1_mem.csv", "1_run.csv"}
			for _, i := range traceFiles {
				_, err := os.Stat(homedir + "/" + loaderTracePath + i)
				if err != nil {
					t.Errorf("Trace file %s does not exist: %s", homedir+"/"+loaderTracePath+i, err)
				}
			}
		})

	}
}
