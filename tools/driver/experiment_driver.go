package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"math"
	"os"
	"path/filepath"
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

	IsPartiallyPanic            bool   `json:"IsPartiallyPanic"`
	EnableZipkinTracing         bool   `json:"EnableZipkinTracing"`
	EnableMetricsScrapping      bool   `json:"EnableMetricsScrapping"`
	MetricScrapingPeriodSeconds int    `json:"MetricScrapingPeriodSeconds"`
	AutoscalingMetric           string `json:"AutoscalingMetric"`

	GRPCConnectionTimeoutSeconds int `json:"GRPCConnectionTimeoutSeconds"`
	GRPCFunctionTimeoutSeconds   int `json:"GRPCFunctionTimeoutSeconds"`
}

type loaderConfig struct {
	configPath          string // path to JSON config file which loader uses
	loaderConfiguration LoaderConfiguration
	functions           int
}

type Driver struct {
	Username                    string   `json:"username"`
	ExperimentName              string   `json:"experimentName"`
	LoaderAddresses             []string `json:"loaderAddresses"`
	clients                     []*simplessh.Client
	BeginningFuncNum            int    `json:"beginningFuncNum"`
	StepSizeFunc                int    `json:"stepSizeFunc"`
	MaxFuncNum                  int    `json:"maxFuncNum"`
	ExperimentDuration          int    `json:"experimentDuration"`
	WarmupDuration              int    `json:"warmupDuration"`
	WorkerNodeNum               int    `json:"workerNodeNum"`
	LocalTracePath              string `json:"localTracePath"`
	LoaderTracePath             string `json:"loaderTracePath"`
	OutputDir                   string `json:"outputDir"`
	YAMLSelector                string `json:"YAMLSelector"`
	IATDistribution             string `json:"IATDistribution"`
	LoaderOutputPath            string `json:"loaderOutputPath"`
	PartiallyPanic              bool   `json:"partiallyPanic"`
	EnableZipkinTracing         bool   `json:"EnableZipkinTracing"`
	EnableMetricsScrapping      bool   `json:"EnableMetricsScrapping"`
	MetricScrapingPeriodSeconds int    `json:"MetricScrapingPeriodSeconds"`
	SeparateIATGeneration       bool   `json:"separateIATGeneration"`
	AutoscalingMetric           string `json:"AutoscalingMetric"`
	loaderConfig                loaderConfig
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x < y {
		return y
	}
	return x
}

func NewDriver(configFile string) *[]Driver {
	driverConfig, _ := os.ReadFile(configFile)
	var driver Driver
	var drivers []Driver
	log.Debugf("Unmarshaling config file: %s", configFile)
	err := json.Unmarshal(driverConfig, &driver)
	if err != nil {
		log.Fatalf("Failed to unmarshal the driver config file: %s", err)
	}
	var loaderConfigs []loaderConfig
	idx := 0
	for i := driver.BeginningFuncNum; i <= driver.MaxFuncNum; i += max(1, driver.StepSizeFunc) {
		loaderConfigs = append(loaderConfigs, driver.createLoaderConfig(i))
		drivers = append(drivers, driver)
		drivers[idx].loaderConfig = loaderConfigs[idx]
		idx++
	}
	return &drivers
}

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
	defer d.clean()
	d.prepareAllLoaders()
	d.startLoad()
	d.collectStats()
	d.aggregateStats()
}

func (d *Driver) createLoaderConfig(functionNumber int) loaderConfig {
	configFile := "loaderConfig_" + strconv.Itoa(functionNumber) + ".json"
	log.Debugf("Creating loader config: %s", configFile)
	loaderTracePath := d.LoaderTracePath
	loaderTracePath = strings.TrimPrefix(loaderTracePath, "loader/")
	var loaderConfig loaderConfig
	var configuration LoaderConfiguration
	configuration = LoaderConfiguration{
		Seed:         42,
		YAMLSelector: d.YAMLSelector,
		EndpointPort: 80,

		TracePath:          loaderTracePath,
		OutputPathPrefix:   d.LoaderOutputPath,
		IATDistribution:    d.IATDistribution,
		ExperimentDuration: d.ExperimentDuration,
		WarmupDuration:     d.WarmupDuration,

		IsPartiallyPanic:            d.PartiallyPanic,
		EnableZipkinTracing:         d.EnableZipkinTracing,
		EnableMetricsScrapping:      d.EnableMetricsScrapping,
		MetricScrapingPeriodSeconds: d.MetricScrapingPeriodSeconds,
		AutoscalingMetric:           d.AutoscalingMetric,

		GRPCConnectionTimeoutSeconds: 60,
		GRPCFunctionTimeoutSeconds:   900,
	}
	file, _ := json.MarshalIndent(configuration, "", " ")
	err := os.WriteFile(configFile, file, 0644)
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
	traceFiles := [3]string{
		functions + "_inv_part_" + strconv.Itoa(idx) + ".csv",
		functions + "_mem_part_" + strconv.Itoa(idx) + ".csv",
		functions + "_run_part_" + strconv.Itoa(idx) + ".csv",
	}
	localFilePaths := [4]string{
		filepath.Join(d.LocalTracePath, traceFiles[0]),
		filepath.Join(d.LocalTracePath, traceFiles[1]),
		filepath.Join(d.LocalTracePath, traceFiles[2]),
		d.loaderConfig.configPath,
	}
	remoteFilePaths := [4]string{
		filepath.Join(d.LoaderTracePath, "invocations.csv"),
		filepath.Join(d.LoaderTracePath, "memory.csv"),
		filepath.Join(d.LoaderTracePath, "durations.csv"),
		filepath.Join("loader/cmd/", d.loaderConfig.configPath),
	}
	for i := 0; i < 4; i++ {
		_, err := os.Stat(localFilePaths[i])
		if err != nil {
			log.Fatalf("Local file %s does not exist: %s", localFilePaths[i], err)
		}
		err = client.Upload(localFilePaths[i], remoteFilePaths[i])
		if err != nil {
			log.Fatalf("Failed to upload the file %s to the loader: %s", localFilePaths[i], err)
		}
	}
}

func (d *Driver) partitionTraceFiles() {
	functions := strconv.Itoa(d.loaderConfig.functions)
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
		records := d.readTraceFile(path)
		for idx := range d.LoaderAddresses {
			d.writePartialTraceFile(idx, i, records)
		}
	}
}

func (d *Driver) readTraceFile(path string) [][]string {
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
	return records
}

func (d *Driver) writePartialTraceFile(idx int, file string, records [][]string) {
	functionsPerLoader := float64(d.loaderConfig.functions) / float64(len(d.LoaderAddresses))
	functionsPerLoader = math.Ceil(functionsPerLoader)
	functionsPerLoaderInt := int(functionsPerLoader)
	partPath := strings.TrimSuffix(file, ".csv")
	partPath = partPath + "_part_" + strconv.Itoa(idx) + ".csv"
	tracePart, err := os.Create(filepath.Join(d.LocalTracePath, partPath))
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

func (d *Driver) prepareLoader(client *simplessh.Client, wg *sync.WaitGroup) {
	defer wg.Done()
	out, err := client.Exec("cd loader; source /etc/profile; make build")
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to build the loader: %s", err)
	}
	if d.SeparateIATGeneration {
		out, err = client.Exec("cd loader; source /etc/profile;go run cmd/loader.go -iatGeneration=true -config " + "cmd/" + d.loaderConfig.configPath)
		log.Debug(string(out))
		if err != nil {
			log.Fatalf("Failed to generate IATs: %s", err)
		}
	}
}

func (d *Driver) startLoad() {
	var wg sync.WaitGroup
	clients := d.clients
	wg.Add(len(clients))
	for _, client := range clients {
		go func(client *simplessh.Client) {
			defer wg.Done()
			var cmd string
			if d.SeparateIATGeneration {
				cmd = "go run cmd/loader.go -generated=true -config " + "cmd/" + d.loaderConfig.configPath
			} else {
				cmd = "go run cmd/loader.go -config " + "cmd/" + d.loaderConfig.configPath
			}
			log.Debug("running command: " + cmd)
			out, err := client.Exec("cd loader; source /etc/profile;" + cmd)
			log.Debug(string(out))
			if err != nil {
				log.Fatalf("Failed to run load generation: %s", err)
			}
		}(client)
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
		go func(client *simplessh.Client, idx int) {
			defer wg.Done()
			durationInt = durationInt + d.WarmupDuration + 1 // add warmup and profiling duration
			duration := strconv.Itoa(durationInt)
			metrics := []string{"duration"}
			if d.loaderConfig.loaderConfiguration.EnableMetricsScrapping {
				metrics = append(metrics, "cluster_usage", "kn_stats")
			}
			for _, metric := range metrics {
				fileLocal := "experiment_" + metric + "_" + duration
				fileRemote := filepath.Join("loader/", d.LoaderOutputPath+"_"+metric+"_"+duration+".csv")
				localPath := filepath.Join(d.OutputDir, experiment)
				log.Debug("making local directory " + localPath)
				err := os.MkdirAll(localPath, os.ModePerm)
				if err != nil {
					log.Fatalf("Creating the local directory %s failed: %s", localPath, err)
				}
				log.Debug("downloading " + fileRemote)
				err = client.Download(fileRemote, filepath.Join(localPath, fileLocal+"_"+functions+"functions_part_"+strconv.Itoa(idx)+".csv"))
				if err != nil {
					log.Fatalf("Downloading the experiment results failed: %s", err)
				}
			}
		}(client, idx)
	}
	wg.Wait()
}

func (d *Driver) aggregateStats() {
	path := filepath.Join(d.OutputDir, d.ExperimentName, "/experiment_duration_"+strconv.Itoa(d.ExperimentDuration+d.WarmupDuration+1))
	path = path + "_" + strconv.Itoa(d.loaderConfig.functions) + "functions_part_0.csv"
	// path to the results file from the first loader, which should always exist
	// If there is only 1 loader, then this is the only results file.
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
	// create aggregated result file, into which we will write the rows from each results file.
	if err != nil {
		log.Fatalf("failed creating aggregated result file %s with error: %s", resultPath, err)
	}
	defer resultFile.Close()
	w := csv.NewWriter(resultFile)
	defer w.Flush()
	err = w.WriteAll(records)
	// writes all rows from the first results file into the aggregated file. This includes the header.
	// If we have only 1 loader, then we can stop here, as there is only 1 result file.
	// Otherwise, we'll iterate over the clients from the other loaders.
	if err != nil {
		log.Fatalf("error writing record to file: %s", err)
	}
	for i := 1; i < len(d.clients); i++ {
		path = filepath.Join(d.OutputDir, d.ExperimentName, "/experiment_duration_"+strconv.Itoa(d.ExperimentDuration+d.WarmupDuration+1))
		path = path + "_" + strconv.Itoa(d.loaderConfig.functions) + "functions_part_" + strconv.Itoa(i) + ".csv"
		// path to the i-th result file (the 0-th one has already been read and written into the aggregated file)
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
			// Discard the first row from the result file, as that is the header, we only want to keep the actual
			// content and write it into the aggregated file.
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
		go func(client *simplessh.Client) {
			defer wg.Done()
			log.Debug("cleaning up")
			out, err := client.Exec("cd loader; source /etc/profile; make clean")
			log.Debug(string(out))
			if err != nil {
				log.Fatalf("Failed to clean the loader state after the experiment: %s", err)
			}
		}(client)
	}
	wg.Wait()
}
