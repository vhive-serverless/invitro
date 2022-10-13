package main

import (
	"encoding/json"
	"flag"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
	"strconv"
	"sync"
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

type loaderConfig struct {
	configPath          string // path to JSON config file which loader uses
	loaderConfiguration LoaderConfiguration
	functions           int
}

type Driver struct {
	Username           string   `json:"username"`
	ExperimentName     string   `json:"experimentName"`
	LoaderAddresses    []string `json:"loaderAddresses"`
	clients            []*simplessh.Client
	BeginningFuncNum   int    `json:"beginningFuncNum"`
	StepSizeFunc       int    `json:"stepSizeFunc"`
	MaxFuncNum         int    `json:"maxFuncNum"`
	ExperimentDuration int    `json:"experimentDuration"`
	WorkerNodeNum      int    `json:"workerNodeNum"`
	LocalTracePath     string `json:"localTracePath"`
	OutputDir          string `json:"outputDir"`
	loaderConfig       loaderConfig
}

func NewDriver(configFile string) *[]Driver {
	driverConfig, _ := ioutil.ReadFile(configFile)
	var d Driver
	var drivers []Driver
	err := json.Unmarshal(driverConfig, &d)
	if err != nil {
		log.Fatalf("Failed tu unmarshal driver config file: %s", err)
	}
	var loaderConfigs []loaderConfig
	idx := 0
	for i := d.BeginningFuncNum; i <= d.MaxFuncNum; i += d.StepSizeFunc {
		loaderConfigs = append(loaderConfigs, d.createLoaderConfig(i))
		drivers = append(drivers, d)
		drivers[idx].loaderConfig = loaderConfigs[idx]
		idx++
	}
	return &drivers
}

const (
	loaderTracePath = "loader/data/traces/"
)

func main() {
	var (
		driverConfigFile = flag.String("c", "driverConfig.json", "JSON config file for the driver")
		debugLevel       = flag.String("d", "info", "Debug level: info, debug")
	)
	flag.Parse()
	log.SetOutput(os.Stdout)
	switch *debugLevel {
	case "info":
		log.SetLevel(log.InfoLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode is enabled")
	}
	drivers := NewDriver(*driverConfigFile)
	startExperimentDrivers(drivers)
}

func startExperimentDrivers(drivers *[]Driver) {
	for _, d := range *drivers {
		d.RunSingleExperiment()
	}
}

func (d *Driver) RunSingleExperiment() {
	d.clients = connectToLoader(d.LoaderAddresses, d.Username)
	for _, client := range d.clients {
		defer client.Close()
	}
	d.prepareAllLoaders()
	d.startLoad()
	d.collectStats()
	d.aggregateStats()
	d.clean()
}

func (d *Driver) createLoaderConfig(functionNumber int) loaderConfig {
	configFile := "loaderConfig_" + strconv.Itoa(functionNumber) + ".json"
	var loaderConfig loaderConfig
	var configuration LoaderConfiguration
	configuration = LoaderConfiguration{
		Seed:         42,
		YAMLSelector: "container",
		EndpointPort: 80,

		TracePath:          "data/traces/" + d.ExperimentName + "/",
		OutputPathPrefix:   "data/out/experiment",
		IATDistribution:    "exponential",
		ExperimentDuration: d.ExperimentDuration,
		WarmupDuration:     10,

		IsPartiallyPanic:       false,
		EnableZipkinTracing:    false,
		EnableMetricsScrapping: false,

		GRPCConnectionTimeoutSeconds: 60,
		GRPCFunctionTimeoutSeconds:   900,
	}
	file, _ := json.MarshalIndent(configuration, "", " ")
	err := ioutil.WriteFile(configFile, file, 0644)
	if err != nil {
		log.Fatalf("Writing the loader config file failed: %s", err)
	}
	loaderConfig.configPath = configFile
	loaderConfig.functions = functionNumber
	loaderConfig.loaderConfiguration = configuration
	return loaderConfig
}

func connectToLoader(loaderAddresses []string, username string) []*simplessh.Client {
	clients := make([]*simplessh.Client, len(loaderAddresses))
	for _, loaderAdr := range loaderAddresses {
		client, err := simplessh.ConnectWithAgent(loaderAdr, username)
		if err != nil {
			log.Fatalf("Connecting to the loader with address %s failed: %s", loaderAdr, err)
		}
		clients = append(clients, client)
	}
	return clients
}

func (d *Driver) prepareAllLoaders() {
	clients := d.clients
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for _, c := range clients {
		go d.prepareLoader(c, &wg)
	}
	wg.Wait()
}

func (d *Driver) prepareLoader(client *simplessh.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	functions := strconv.Itoa(d.loaderConfig.functions)
	tracePath := d.LocalTracePath
	_, err := os.Stat(tracePath)
	if err != nil {
		log.Fatalf("Trace directory %s does not exist: %s", tracePath, err)
	}
	traceFiles := [3]string{functions + "_inv.csv", functions + "_mem.csv", functions + "_run.csv"}
	// TODO: transfer JSON config file
	for _, i := range traceFiles {
		_, err := os.Stat(tracePath + "/" + i)
		if err != nil {
			log.Fatalf("Trace file %s does not exist: %s", tracePath+"/"+i, err)
		}
		err = client.Upload(tracePath+"/"+i, loaderTracePath+i)
		if err != nil {
			log.Fatalf("Failed to upload the trace files to the loader: %s", err)
		}
	}
	out, err := client.Exec("cd loader; source /etc/profile; make build")
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to build the loader: %s", err)
	}
	// TODO: Run iat generation
	out, err = client.Exec("cd loader; source /etc/profile;go run cmd/load.go -iatGeneration -config " + d.loaderConfig.configPath)
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to generate IATs: %s", err)
	}
}

func (d *Driver) startLoad() {
	// TODO: change command to account for config file
	// var experimentDuration, numberOfNodes, functions string
	var wg sync.WaitGroup
	clients := d.clients
	wg.Add(len(clients))
	for _, client := range clients {
		go func() {
			defer wg.Done()
			// cmd := "go run cmd/load.go -experimentDuration " + experimentDuration + " -cluster " + numberOfNodes + " -server trace " +
			// 	"-print info -sample " + functions + " -warmup"
			cmd := "go run cmd/load.go -config " + d.loaderConfig.configPath
			log.Debug("running command: " + cmd)
			out, err := client.Exec("cd loader; source /etc/profile;" + cmd)
			log.Debug(string(out))
			if err != nil {
				log.Fatalf("Failed to run load generation: %s", err)
			}
		}()
	}
	wg.Wait()
}

func (d *Driver) collectStats() {
	experiment := d.ExperimentName
	durationInt := d.ExperimentDuration
	functions := strconv.Itoa(d.loaderConfig.functions)
	clients := d.clients
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for _, client := range clients {
		go func() {
			defer wg.Done()
			durationInt = durationInt + 15 // add profiling + + warmup experimentDuration
			duration := strconv.Itoa(durationInt)
			fileLocal := "exec_duration_" + duration + ".csv"
			fileRemote := "loader/data/out/exec_duration_" + duration + ".csv"
			localPath := d.OutputDir + "/" + experiment
			log.Debug("making local directory " + localPath)
			err := os.MkdirAll(localPath, os.ModePerm)
			if err != nil {
				log.Fatalf("Creating the local directory %s failed: %s", localPath, err)
			}
			log.Debug("downloading " + fileRemote)
			err = client.Download(fileRemote, localPath+"/"+fileLocal+"_"+functions+"functions")
			if err != nil {
				log.Fatalf("Downloading the experiment results failed: %s", err)
			}
		}()
	}
	wg.Wait()
}

func (d *Driver) aggregateStats() {
	// TODO: concatenate result files that were collected by collectStats()
	var err error
	if err != nil {
		log.Fatalf("Failed to aggregate the results from multiple loaders: %s", err)
	}
}

func (d *Driver) clean() {
	var wg sync.WaitGroup
	clients := d.clients
	wg.Add(len(clients))
	for _, client := range clients {
		go func() {
			defer wg.Done()
			log.Debug("cleaning up")
			out, err := client.Exec("cd loader; source /etc/profile; make clean")
			log.Debug(string(out))
			if err != nil {
				log.Fatalf("Failed to clean the loader state after the experiment: %s", err)
			}
		}()
	}
}
