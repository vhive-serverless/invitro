package runner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"

	log "github.com/sirupsen/logrus"
)

const (
	LOADER_PATH = "cmd/loader.go"
	TIME_FORMAT = "Jan_02_1504"
	EXPERIMENT_TEMP_CONFIG_PATH = "tools/multi_loader/current_running_config.json"
	NUM_OF_RETRIES = 2
)

type MultiLoaderRunner struct {
    MultiLoaderConfig common.MultiLoaderConfiguration
    NodeGroup common.NodeGroup
    DryRunSuccess bool
	Verbosity	string
	IatGeneration	bool
	Generated	bool
	DryRun bool
	Platform string
}

// init multi loader runner
func NewMultiLoaderRunner(configPath string, verbosity string, iatGeneration bool, generated bool) (*MultiLoaderRunner, error) {
    multiLoaderConfig := config.ReadMultiLoaderConfigurationFile(configPath)

	// validate configuration
	common.CheckMultiLoaderConfig(multiLoaderConfig)
	
	// determine platform
	platform := determinePlatform(multiLoaderConfig)

    runner := MultiLoaderRunner{
        MultiLoaderConfig: multiLoaderConfig,
        DryRunSuccess: true,
		Verbosity: verbosity,
		IatGeneration: iatGeneration,
		Generated: generated,
		DryRun: false,
		Platform: platform,
    }
	
	return &runner, nil
}

func determinePlatform(multiLoaderConfig common.MultiLoaderConfiguration) string {
	// Determine platform
	baseConfigByteValue, err := os.ReadFile(multiLoaderConfig.BaseConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	var loaderConfig config.LoaderConfiguration
	// Unmarshal base configuration
	if err = json.Unmarshal(baseConfigByteValue, &loaderConfig); err != nil {
		log.Fatal(err)
	}
	return loaderConfig.Platform
}

func (d *MultiLoaderRunner) RunDryRun() {
    log.Info("Running dry run")
    d.DryRun = true
    d.run()
}

func (d *MultiLoaderRunner) RunActual() {
    log.Info("Running actual experiments")
    d.DryRun = false
    d.run()
}

func (d *MultiLoaderRunner) run(){
	// Run global prescript
	common.RunScript(d.MultiLoaderConfig.PreScript)
	// Iterate over studies and run them
	for _, study := range d.MultiLoaderConfig.Studies {
		log.Info("Setting up experiment: ", study.Name)
		// Run pre script
		common.RunScript(study.PreScript)	

		// Unpack study to a list of studies with different loader configs
		sparseExperiments := d.unpackStudy(study)

		// Iterate over sparse experiments, prepare and run
		for _, experiment := range sparseExperiments {
			if d.DryRun{
				log.Info("Dry Running: ", experiment.Name)
			}
			// Prepare experiment: merge with base config, create output dir and write merged config to temp file
			d.prepareExperiment(experiment)

			err := d.runExperiment(experiment)

			// Perform cleanup
			d.performCleanup()

			// Check if should continue this study
			if err != nil {
				log.Info("Experiment failed: ", experiment.Name, ". Skipping remaining experiments in study...")
				break
			}
		}
		// Run post script
		common.RunScript(study.PostScript)
		if len(sparseExperiments) > 1 && !d.DryRun{
			log.Info("All experiments for ", study.Name, " completed")
		}
	}
	// Run global postscript
	common.RunScript(d.MultiLoaderConfig.PostScript)
}

/** 
* As a study can have multiple experiments, this function will unpack the study
* but first by duplicating the study to multiple studies with different values 
* in the config field. Those values will override the base loader config later
*/
func (d *MultiLoaderRunner) unpackStudy(experiment common.LoaderStudy) []common.LoaderStudy {
	log.Info("Unpacking experiment ", experiment.Name)
	var experiments []common.LoaderStudy

	// if user specified a trace directory
	if experiment.TracesDir != "" {
		experiments = d.unpackFromTraceDir(experiment)
	// user define trace format and values instead of directory
	} else if experiment.TracesFormat != "" && len(experiment.TraceValues) > 0 {
		experiments = d.unpackFromTraceValues(experiment)
	} else {
		// Theres only one experiment in the study
		experiments = d.unpackSingleExperiment(experiment)
	}

	return experiments
}

func (d *MultiLoaderRunner) unpackFromTraceDir(study common.LoaderStudy) []common.LoaderStudy {
	var experiments []common.LoaderStudy
	files, err := os.ReadDir(study.TracesDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		newExperiment := d.createNewStudy(study, file.Name())
		experiments = append(experiments, newExperiment)
	}
	return experiments
}

func (d *MultiLoaderRunner) unpackFromTraceValues(study common.LoaderStudy) []common.LoaderStudy {
	var experiments []common.LoaderStudy
	for _, traceValue := range study.TraceValues {
		tracePath := strings.Replace(study.TracesFormat, common.TraceFormatString, fmt.Sprintf("%v", traceValue), -1)
		fileName := path.Base(tracePath)
		newExperiment := d.createNewStudy(study, fileName)
		newExperiment.Config["TracePath"] = tracePath
		newExperiment.Name += "_" + fileName
		experiments = append(experiments, newExperiment)
	}
	return experiments
}

func (d *MultiLoaderRunner) unpackSingleExperiment(study common.LoaderStudy) []common.LoaderStudy {
	var experiments []common.LoaderStudy
	pathDir := ""
	if study.Config["OutputPathPrefix"] != nil {
		pathDir = path.Dir(study.Config["OutputPathPrefix"].(string))
	} else {
		pathDir = study.OutputDir
	}
	study.OutputDir = pathDir
	newExperiment := d.createNewStudy(study, study.Name)
	experiments = append(experiments, newExperiment)
	return experiments
}

func (d *MultiLoaderRunner) createNewStudy(study common.LoaderStudy, fileName string) common.LoaderStudy {
	newStudy, err := common.DeepCopy(study)
	if err != nil {
		log.Fatal(err)
	}

	dryRunAdditionalPath := ""
	if d.DryRun {
		dryRunAdditionalPath = "dry_run"
	}
	newStudy.Config["OutputPathPrefix"] = path.Join(
		study.OutputDir,
		study.Name,
		dryRunAdditionalPath,
		time.Now().Format(TIME_FORMAT)+"_"+fileName,
		fileName,
	)
	d.addCommandFlags(newStudy)
	return newStudy
}

func (d *MultiLoaderRunner) addCommandFlags(study common.LoaderStudy) {
	// Add flags to experiment config
	if study.Verbosity == "" {
		study.Verbosity = d.Verbosity
	}
	if !study.IatGeneration {
		study.IatGeneration = d.IatGeneration
	}
	if !study.Generated {
		study.Generated = d.Generated
	} 
}

/**
* Prepare experiment by merging with base config, creating output directory and writing experiment config to temp file
*/
func (d *MultiLoaderRunner) prepareExperiment(experiment common.LoaderStudy) {
	log.Info("Preparing ", experiment.Name)
	// Merge base configs with experiment configs
	experimentConfig := d.mergeConfigurations(d.MultiLoaderConfig.BaseConfigPath, experiment)
    
	// Create output directory
	outputDir := path.Dir(experimentConfig.OutputPathPrefix)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatal(err)
	}
	// Write experiment configs to temp file
	d.writeExperimentConfigToTempFile(experimentConfig, EXPERIMENT_TEMP_CONFIG_PATH)
}

/**
* Merge base configs with partial loader configs
*/
func (d *MultiLoaderRunner) mergeConfigurations(baseConfigPath string, experiment common.LoaderStudy) config.LoaderConfiguration {
	// Read base configuration
	baseConfigByteValue, err := os.ReadFile(baseConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Debug("Experiment configuration ", experiment.Config)
	
	var mergedConfig config.LoaderConfiguration
	// Unmarshal base configuration
	if err = json.Unmarshal(baseConfigByteValue, &mergedConfig); err != nil {
		log.Fatal(err)
	}

	log.Debug("Base configuration ", mergedConfig)
	
	// merge experiment config onto base config
	experimentConfigBytes, _ := json.Marshal(experiment.Config)
	if err = json.Unmarshal(experimentConfigBytes, &mergedConfig); err != nil {
		log.Fatal(err)
	}
	log.Debug("Merged configuration ", mergedConfig)

	return mergedConfig
}

func (d *MultiLoaderRunner) writeExperimentConfigToTempFile(experimentConfig config.LoaderConfiguration, fileWritePath string) {
	experimentConfigBytes, _ := json.Marshal(experimentConfig)
	err := os.WriteFile(fileWritePath, experimentConfigBytes, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func (d *MultiLoaderRunner) runExperiment(experiment common.LoaderStudy) error {
	log.Info("Running ", experiment.Name)
	log.Debug("Experiment configuration ", experiment.Config)

	// Create the log file
	logFilePath := path.Join(path.Dir(experiment.Config["OutputPathPrefix"].(string)), "loader.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	for i := 0; i < NUM_OF_RETRIES; i++ {
		// Run loader.go with experiment configs
		if err := d.executeLoaderCommand(experiment, logFile); err != nil {
			log.Error(err)
			log.Error("Experiment failed: ", experiment.Name)
			logFile.WriteString("Experiment failed: " + experiment.Name + ". Error: " + err.Error() + "\n")
			if i == 0 && !d.DryRun {
				log.Info("Retrying experiment ", experiment.Name)
				logFile.WriteString("==================================RETRYING==================================\n")
				experiment.Verbosity = "debug"
			} else{
				// Experiment failed set dry run flag to false
				d.DryRunSuccess = false
				log.Error("Check log file for more information: ", logFilePath)
				// should not continue with experiment
				return err
			}
			continue
		}else{
			break
		}
	}
	log.Info("Completed ", experiment.Name)
	return nil
}

func (d *MultiLoaderRunner) executeLoaderCommand(experiment common.LoaderStudy, logFile *os.File) error {
	cmd := exec.Command("go", "run", LOADER_PATH,
		"--config="+EXPERIMENT_TEMP_CONFIG_PATH,
		"--verbosity="+experiment.Verbosity,
		"--iatGeneration="+strconv.FormatBool(experiment.IatGeneration),
		"--generated="+strconv.FormatBool(experiment.Generated),
		"--dryRun="+strconv.FormatBool(d.DryRun))

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	
	if err := cmd.Start(); err != nil {
		return err
	}
	
	go d.logLoaderStdOutput(stdout, logFile)
	go d.logLoaderStdError(stderr, logFile)
	
	return cmd.Wait()
}

func (d *MultiLoaderRunner) logLoaderStdOutput(stdPipe io.ReadCloser, logFile *os.File) {
	scanner := bufio.NewScanner(stdPipe)
	for scanner.Scan() {
		m := scanner.Text()
		// write to log file
		logFile.WriteString(m + "\n")
		
		// Log key information
		if m == "" {
			continue
		}
		logType := common.ParseLogType(m)
		message := common.ParseLogMessage(m)
		
		switch logType {
		case "debug":
			log.Debug(message)
		case "trace":
			log.Trace(message)
		default:
			if strings.Contains(message, "Number of successful invocations:") || strings.Contains(message, "Number of failed invocations:") {
				log.Info(strings.ReplaceAll(strings.ReplaceAll(message, "\\t", " ",), "\\n", ""))
			}
		}
	}
}

func (d *MultiLoaderRunner) logLoaderStdError(stdPipe io.ReadCloser, logFile *os.File) {
	scanner := bufio.NewScanner(stdPipe)
	for scanner.Scan() {
		m := scanner.Text()
		// write to log file
		logFile.WriteString(m + "\n")
		
		if m == "" {
			continue
		}
		log.Error(m)
	}
}

func (d *MultiLoaderRunner) performCleanup() {
	log.Info("Runnning Cleanup")
	// Run make clean
	if err := exec.Command("make", "clean").Run(); err != nil {
		log.Error(err)
	}
	log.Info("Cleanup completed")
	// Remove temp file
	os.Remove(EXPERIMENT_TEMP_CONFIG_PATH)
}
