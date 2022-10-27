package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"math"
	"os"
	"strconv"
	"strings"
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

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func NewDriver(configFile string) *[]Driver {
	driverConfig, _ := ioutil.ReadFile(configFile)
	var d Driver
	var drivers []Driver
	log.Debugf("Unmarshaling config file: %s", configFile)
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
		log.Debugf("Starting experiment: %s", d.ExperimentName)
		d.RunSingleExperiment()
	}
}

func (d *Driver) RunSingleExperiment() {
	d.clients = connectToLoader(d.LoaderAddresses, d.Username)
	for _, client := range d.clients {
		defer client.Close()
	}
	log.Debug("connected to all loaders")
	d.prepareAllLoaders()
	d.startLoad()
	d.collectStats()
	d.aggregateStats()
	d.clean()
}

func (d *Driver) createLoaderConfig(functionNumber int) loaderConfig {
	configFile := "loaderConfig_" + strconv.Itoa(functionNumber) + ".json"
	log.Debugf("Creating loader config: %s", configFile)
	var loaderConfig loaderConfig
	var configuration LoaderConfiguration
	configuration = LoaderConfiguration{
		Seed:         42,
		YAMLSelector: "container",
		EndpointPort: 80,

		TracePath:          "data/traces",
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
	for idx, loaderAdr := range loaderAddresses {
		log.Debugf("connecting to loader: %s", loaderAdr)
		client, err := simplessh.ConnectWithAgent(loaderAdr, username)
		//client, err := simplessh.ConnectWithKeyFile(loaderAdr, username, "/home/mihajlo/.ssh/id_ed25519")
		if err != nil {
			log.Fatalf("Connecting to the loader with address %s failed: %s", loaderAdr, err)
		}
		log.Debugf("connected to loader: %s", loaderAdr)
		clients[idx] = client
	}
	return clients
}

func (d *Driver) prepareAllLoaders() {
	log.Debugf("Preparing loader for experiment: %s", d.ExperimentName)
	clients := d.clients
	log.Debugf("Partitioning trace files")
	d.partitionTraceFiles()
	var wg sync.WaitGroup
	wg.Add(len(clients))
	for idx, c := range clients {
		log.Debugf("Transfering files to loader %d", idx)
		go d.transferFilesToLoader(c, &wg, idx)
	}
	wg.Wait()
	wg.Add(len(clients))
	for _, c := range clients {
		go d.prepareLoader(c, &wg)
	}
	wg.Wait()
}

func (d *Driver) transferFilesToLoader(client *simplessh.Client, wg *sync.WaitGroup, idx int) {
	defer wg.Done()
	functions := strconv.Itoa(d.loaderConfig.functions)
	tracePath := d.LocalTracePath
	traceFiles := [3]string{
		functions + "_inv_part_" + strconv.Itoa(idx) + ".csv",
		functions + "_mem_part_" + strconv.Itoa(idx) + ".csv",
		functions + "_run_part_" + strconv.Itoa(idx) + ".csv",
	}
	for index, i := range traceFiles {
		_, err := os.Stat(tracePath + "/" + i)
		if err != nil {
			log.Fatalf("Trace file %s does not exist: %s", tracePath+"/"+i, err)
		}
		var loaderTraceFile string
		switch index {
		case 0:
			loaderTraceFile = "invocations.csv"
		case 1:
			loaderTraceFile = "memmory.csv"
		case 2:
			loaderTraceFile = "durations.csv"

		}
		err = client.Upload(tracePath+"/"+i, loaderTracePath+loaderTraceFile)
		if err != nil {
			log.Fatalf("Failed to upload the trace files to the loader: %s", err)
		}
	}
	_, err := os.Stat(d.loaderConfig.configPath)
	if err != nil {
		log.Fatalf("Loader config file %s does not exist: %s", d.loaderConfig.configPath, err)
	}
	err = client.Upload(d.loaderConfig.configPath, "loader/cmd/"+d.loaderConfig.configPath)
	if err != nil {
		log.Fatalf("Failed to upload the loader config files to the loader: %s", err)
	}
}

func (d *Driver) partitionTraceFiles() {
	functions := strconv.Itoa(d.loaderConfig.functions)
	functionsPerLoader := float64(d.loaderConfig.functions) / float64(len(d.LoaderAddresses))
	functionsPerLoader = math.Ceil(functionsPerLoader)
	functionsPerLoaderInt := int(functionsPerLoader)
	tracePath := d.LocalTracePath
	_, err := os.Stat(tracePath)
	if err != nil {
		log.Fatalf("Trace directory %s does not exist: %s", tracePath, err)
	}
	traceFiles := [3]string{
		functions + "_inv.csv",
		functions + "_mem.csv",
		functions + "_run.csv",
	}
	for _, i := range traceFiles {
		path := tracePath + "/" + i
		_, err := os.Stat(path)
		if err != nil {
			log.Fatalf("Trace file %s does not exist: %s", path, err)
		}
		f, err := os.Open(path)
		if err != nil {
			log.Fatalf("Failed to read trace file %s with error: %s", path, err)
		}
		defer f.Close()

		reader := csv.NewReader(f)
		records, err := reader.ReadAll()
		if err != nil {
			log.Fatalf("Invalid trace structure for file %s with error: %s", path, err)
		}
		for idx, _ := range d.LoaderAddresses {
			partPath := strings.TrimSuffix(i, ".csv")
			partPath = partPath + "_part_" + strconv.Itoa(idx) + ".csv"
			tracePart, err := os.Create(tracePath + "/" + partPath)
			if err != nil {
				log.Fatalf("failed creating trace file %s with error: %s", partPath, err)
			}
			defer tracePart.Close()
			w := csv.NewWriter(tracePart)
			defer w.Flush()
			err = w.Write(records[0])
			if err != nil {
				log.Fatalf("error writing record to file: %s", err)
			}
			startIdx := 1 + idx*functionsPerLoaderInt
			endIdx := min(startIdx+functionsPerLoaderInt, len(records))
			for i := startIdx; i < endIdx; i++ {
				err = w.Write(records[i])
				if err != nil {
					log.Fatalf("error writing record to file: %s", err)
				}
			}
		}
	}
}

func (d *Driver) prepareLoader(client *simplessh.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	out, err := client.Exec("cd loader; source /etc/profile; make build")
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to build the loader: %s", err)
	}
	out, err = client.Exec("cd loader; source /etc/profile;go run cmd/loader.go -iatGeneration=true -config " + "cmd/" + d.loaderConfig.configPath)
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to generate IATs: %s", err)
	}
}

func (d *Driver) startLoad() {
	var wg sync.WaitGroup
	clients := d.clients
	wg.Add(len(clients))
	for _, client := range clients {
		go func() {
			defer wg.Done()
			cmd := "go run cmd/loader.go -generated=true -config " + "cmd/" + d.loaderConfig.configPath
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
	for idx, client := range clients {
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
			err = client.Download(fileRemote, localPath+"/"+fileLocal+"_"+functions+"functions_part_"+strconv.Itoa(idx)+".csv")
			if err != nil {
				log.Fatalf("Downloading the experiment results failed: %s", err)
			}
		}()
	}
	wg.Wait()
}

func (d *Driver) aggregateStats() {
	path := d.OutputDir + "/" + d.ExperimentName + "/exec_duration_" + strconv.Itoa(d.ExperimentDuration+15)
	path = path + "_" + strconv.Itoa(d.loaderConfig.functions) + "functions_part_0.csv"
	_, err := os.Stat(path)
	if err != nil {
		log.Fatalf("Result file %s does not exist: %s", path, err)
	}
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("Failed to read result file %s with error: %s", path, err)
	}
	defer f.Close()
	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Invalid result structure for file %s with error: %s", path, err)
	}
	resultPath := strings.TrimSuffix(path, "_part_0.csv")
	resultPath = resultPath + "_aggregated.csv"
	resultFile, err := os.Create(resultPath)
	if err != nil {
		log.Fatalf("failed creating trace file %s with error: %s", resultPath, err)
	}
	defer resultFile.Close()
	w := csv.NewWriter(resultFile)
	defer w.Flush()
	err = w.WriteAll(records)
	if err != nil {
		log.Fatalf("error writing record to file: %s", err)
	}
	for i := 1; i < len(d.clients); i++ {
		path = d.OutputDir + "/" + d.ExperimentName + "/exec_duration_" + strconv.Itoa(d.ExperimentDuration+15)
		path = path + "_" + strconv.Itoa(d.loaderConfig.functions) + "functions_part_" + strconv.Itoa(i) + ".csv"
		_, err := os.Stat(path)
		if err != nil {
			log.Fatalf("Result file %s does not exist: %s", path, err)
		}
		f, err := os.Open(path)
		if err != nil {
			log.Fatalf("Failed to read result file %s with error: %s", path, err)
		}
		defer f.Close()
		reader := csv.NewReader(f)
		records, err := reader.ReadAll()
		for i := 1; i < len(records); i++ {
			err = w.Write(records[i])
			if err != nil {
				log.Fatalf("error writing record to file: %s", err)
			}
		}
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
