package main

import (
	"flag"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"os"
	"strconv"
	"sync"
)

func main() {
	var (
		name      = flag.String("n", "sweep", "Name of the experiment")
		outputDir = flag.String("o", "raw_measurements", "Path to the directory for output files")
		tracePath = flag.String("t", "traces", "Path to the directory with trace files")
		loaderAdr = flag.String("l", "155.98.36.75", "IP address of the loader node")
		beginning = flag.Int("b", 10, "Starting number of functions")
		step      = flag.Int("s", 10, "Step size for increase in number of functions")
		max       = flag.Int("m", 100, "Maximum number of functions")
		duration  = flag.Int("dur", 10, "Duration of measurement phase of experiment")
		nodes     = flag.Int("nodes", 1, "Number of worker nodes")
		//https://stackoverflow.com/questions/28322997/how-to-get-a-list-of-values-into-a-flag-in-golang
		debugLevel = flag.String("d", "info", "Debug level: info, debug")
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
	configs := make([]string, 1)
	loaderAddresses := make([]string, 1) // TODO: multiple loaderAddresses for each config
	loaderAddresses = append(loaderAddresses, *loaderAdr)
	for i := *beginning; i <= *max; i += *step {
		configFile, err := createConfig(*name, *nodes, *duration, i)
		if err != nil {
			log.Fatal(err)
		}
		configs = append(configs, configFile)
	}
	for _, cfg := range configs {
		err := SshAndRunExperiment(loaderAddresses, *tracePath, *name, *outputDir, cfg) // TODO: pass addresses of all loaders here
		if err != nil {
			log.Fatal(err)
		}
	}
}

func SshAndRunExperiment(loaderAddresses []string, tracePath string, name string, outputDir string, configFile string) error {
	var client *simplessh.Client
	var err error
	clients := make([]*simplessh.Client, 1)

	for _, v := range loaderAddresses {
		client, err = connectToLoader(v)
		if err != nil {
			return err
		}
		defer client.Close()
		clients = append(clients, client)
	}

	var wg sync.WaitGroup
	wg.Add(len(clients))
	for _, c := range clients {
		go prepareLoader(c, configFile, tracePath, &wg)
	}
	wg.Wait()

	wg.Add(len(clients))
	for _, c := range clients {
		go startLoad(c, configFile, &wg)
	}
	wg.Wait()

	for _, c := range clients {
		err = collectStats(name, c, outputDir, configFile)
		if err != nil {
			log.Info("Make sure that the experiment directory exists locally")
			return err
		}
	}
	err = aggregateStats(name, outputDir, configFile)
	if err != nil {
		return err
	}
	for _, c := range clients {
		err = cleanUp(c)
		if err != nil {
			return err
		}
	}

	return nil
}

func createConfig(experimentName string, numberOfNodes int, dur int, functions int) (string, error) {
	// TODO: create JSON config file that loader accepts
	var configFile string
	var err error
	return configFile, err
}

func connectToLoader(loaderAdr string) (*simplessh.Client, error) {
	client, err := simplessh.ConnectWithAgent(loaderAdr, "Mihajlo")
	if err != nil {
		return nil, err
	}
	return client, err
}

func prepareLoader(client *simplessh.Client, configFile string, tracePath string, wg *sync.WaitGroup) {
	defer wg.Done()
	var functions string
	// TODO: read out number of functions from configFile
	traceFiles := [3]string{functions + "_inv.csv", functions + "_mem.csv", functions + "_run.csv"}
	// TODO: transfer JSON config file
	for _, i := range traceFiles {
		err := client.Upload(tracePath+"/"+i, "loader/data/traces/"+i)
		if err != nil {
			log.Fatal(err)
		}
	}
	out, err := client.Exec("cd loader; source /etc/profile; make build")
	// TODO: Run iat generation
	log.Debug(string(out))
	if err != nil {
		log.Fatal(err)
	}
}

func startLoad(client *simplessh.Client, configFile string, wg *sync.WaitGroup) {
	defer wg.Done()
	// TODO: change command to account for config file
	var dur string
	var numberOfNodes string
	var functions string

	cmd := "go run cmd/load.go -duration " + dur + " -cluster " + numberOfNodes + " -server trace " +
		"-print info -sample " + functions + " -warmup"
	log.Info("running command: " + cmd)
	out, err := client.Exec("cd loader; source /etc/profile;" + cmd)
	log.Debug(string(out))
	if err != nil {
		log.Fatal(err)
	}
}

func collectStats(name string, client *simplessh.Client, outputDir string, configFile string) error {
	var dur string // TODO: read from configFile
	var functions string

	log.Info("making directory " + name)
	out, err := client.Exec("mkdir -p " + name)
	log.Debug(string(out))
	if err != nil {
		return err
	}
	durInt, err := strconv.Atoi(dur)
	if err != nil {
		return err
	}
	durInt = durInt + 15 // add profiling + + warmup duration
	dur = strconv.Itoa(durInt)
	fileLocal := "exec_duration_" + dur + ".csv"
	fileRemote := "loader/data/out/exec_duration_" + dur + ".csv"
	log.Info("downloading " + fileRemote)
	err = client.Download(fileRemote, outputDir+"/"+name+"/"+fileLocal+"_"+functions+"functions")
	return err
}

func aggregateStats(name string, outputDir string, configFile string) error {
	// TODO: concatenate result files that were collected by collectStats()
	var err error
	return err
}

func cleanUp(client *simplessh.Client) error {
	log.Info("cleaning up")
	out, err := client.Exec("cd loader; source /etc/profile; make clean")
	log.Debug(string(out))
	return err
}
