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
	ml_common "github.com/vhive-serverless/loader/tools/multi_loader/common"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"

	log "github.com/sirupsen/logrus"
)

const (
	LOADER_PATH                 = "cmd/loader.go"
	TIME_FORMAT                 = "Jan_02_1504"
	EXPERIMENT_TEMP_CONFIG_PATH = "tools/multi_loader/current_running_config.json"
	NUM_OF_RETRIES              = 1
)

type MultiLoaderRunner struct {
	MultiLoaderConfig types.MultiLoaderConfiguration
	DryRunSuccess     bool
	Verbosity         string
	DryRun            bool
	Platform          string
	FailFast          bool
}

/**
* Initialise a new MultiLoaderRunner
**/
func NewMultiLoaderRunner(configPath string, verbosity string, failFast bool) (*MultiLoaderRunner, error) {
	multiLoaderConfig := ml_common.ReadMultiLoaderConfigurationFile(configPath)

	// Validate configuration
	ml_common.CheckMultiLoaderConfig(multiLoaderConfig)

	// Determine platform
	platform := ml_common.DeterminePlatformFromConfig(multiLoaderConfig)

	runner := MultiLoaderRunner{
		MultiLoaderConfig: multiLoaderConfig,
		DryRunSuccess:     true,
		Verbosity:         verbosity,
		DryRun:            false,
		Platform:          platform,
		FailFast:          failFast,
	}

	return &runner, nil
}

/**
* Run multi loader with dry flag to check configurations
**/
func (d *MultiLoaderRunner) RunDryRun() {
	log.Info("Running dry runs")
	d.DryRun = true
	d.run()
}

/**
* Run multi loader with actual experiments
**/
func (d *MultiLoaderRunner) RunActual() {
	log.Info("Running actual experiments")
	d.DryRun = false
	d.run()
}

/**
* Main multi loader logic
**/
func (d *MultiLoaderRunner) run() {
	// Run global prescript
	common.RunScript(d.MultiLoaderConfig.PreScript)
	// Iterate over studies and run them
	for si, study := range d.MultiLoaderConfig.Studies {
		log.Debug("Setting up study: ", study.Name)
		// Run pre script
		common.RunScript(study.PreScript)

		// Unpack study to a list of studies with different loader configs
		experimentsPartialConfig := d.unpackStudy(study)

		// Iterate over experiments partial config, prepare by merging with base and run
		for ei, experiment := range experimentsPartialConfig {
			if d.DryRun {
				log.Info(fmt.Sprintf("[Study %d/%d][Experiment %d/%d] Dry running %s", si+1, len(d.MultiLoaderConfig.Studies), ei+1, len(experimentsPartialConfig), experiment.Name))
			} else {
				log.Info(fmt.Sprintf("[Study %d/%d][Experiment %d/%d] Running %s", si+1, len(d.MultiLoaderConfig.Studies), ei+1, len(experimentsPartialConfig), experiment.Name))
			}

			// Prepare experiment: merge with base config, create output dir and write merged config to temp file
			d.prepareExperiment(experiment)

			err := d.runExperiment(experiment)

			// Perform cleanup
			d.performCleanup()

			// Check if should continue this study
			if err != nil {
				log.Error("Experiment failed: ", experiment.Name, ". Skipping remaining experiments in study...")
				break
			}
		}
		// Run post script
		common.RunScript(study.PostScript)
		if len(experimentsPartialConfig) > 1 && !d.DryRun {
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
**/
func (d *MultiLoaderRunner) unpackStudy(study types.LoaderStudy) []types.LoaderExperiment {
	log.Debug("Unpacking study ", study.Name)
	var experiments []types.LoaderExperiment

	if study.TracesDir != "" {
		// User specified a trace directory
		experiments = d.unpackFromTraceDir(study)
	} else if study.TracesFormat != "" && len(study.TraceValues) > 0 {
		// User define trace format and values instead of directory
		experiments = d.unpackFromTraceValues(study)
	} else {
		// Theres only one experiment in the study
		experiments = d.unpackSingleExperiment(study)
	}
	return experiments
}

/**
* Creates experiments for each trace found in the trace directory
**/
func (d *MultiLoaderRunner) unpackFromTraceDir(study types.LoaderStudy) []types.LoaderExperiment {
	var experiments []types.LoaderExperiment
	files, err := os.ReadDir(study.TracesDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, file := range files {
		tracePath := path.Join(study.TracesDir, file.Name())
		newExperiment := d.createExperimentFromStudy(study, file.Name(), tracePath)
		experiments = append(experiments, newExperiment)
	}
	return experiments
}

/**
* Creates experiments for each trace derived from substituting the trace values into the trace format
**/
func (d *MultiLoaderRunner) unpackFromTraceValues(study types.LoaderStudy) []types.LoaderExperiment {
	var experiments []types.LoaderExperiment
	for _, traceValue := range study.TraceValues {
		tracePath := strings.Replace(study.TracesFormat, ml_common.TraceFormatString, fmt.Sprintf("%v", traceValue), -1)
		fileName := path.Base(tracePath)
		newExperiment := d.createExperimentFromStudy(study, fileName, tracePath)
		experiments = append(experiments, newExperiment)
	}
	return experiments
}

/**
* Creates a single experiment suitable for multi loader to run
**/
func (d *MultiLoaderRunner) unpackSingleExperiment(study types.LoaderStudy) []types.LoaderExperiment {
	var experiments []types.LoaderExperiment
	pathDir := ""
	if study.Config["OutputPathPrefix"] != nil {
		pathDir = path.Dir(study.Config["OutputPathPrefix"].(string))
	} else {
		pathDir = study.OutputDir
	}
	study.OutputDir = pathDir
	newExperiment := d.createExperimentFromStudy(study, study.Name, "")
	experiments = append(experiments, newExperiment)
	return experiments
}

/**
* Creates a LoaderExperiment from a given study and updates relevant expereiment fields
 */
func (d *MultiLoaderRunner) createExperimentFromStudy(study types.LoaderStudy, experimentName string, tracePath string) types.LoaderExperiment {
	experiment := types.LoaderExperiment{
		Name:       study.Name + "_" + experimentName,
		OutputDir:  study.OutputDir,
		PreScript:  study.PreScript,
		PostScript: study.PostScript,
	}

	// Deep copy of study config for new experiment
	studyConfig, err := common.DeepCopy(study.Config)
	if err != nil {
		log.Fatal(err)
	}
	// Update config to duplicated study configs
	experiment.Config = studyConfig

	// Update OutputPathPrefix
	dryRunAdditionalPath := ""
	if d.DryRun {
		dryRunAdditionalPath = "dry_run"
	}
	studyConfig["OutputPathPrefix"] = path.Join(
		study.OutputDir,
		study.Name,
		dryRunAdditionalPath,
		time.Now().Format(TIME_FORMAT)+"_"+experimentName,
		experimentName,
	)
	// Add loader command flags
	d.addCommandFlagsToExperiment(experiment)

	// Update trace path
	if tracePath != "" {
		studyConfig["TracePath"] = tracePath
	}
	return experiment
}

func (d *MultiLoaderRunner) addCommandFlagsToExperiment(experiment types.LoaderExperiment) {
	// Add flags to study config
	if experiment.Verbosity == "" {
		experiment.Verbosity = d.Verbosity
	}
	if !experiment.IatGeneration {
		experiment.IatGeneration = d.MultiLoaderConfig.IatGeneration
	}
	if !experiment.Generated {
		experiment.Generated = d.MultiLoaderConfig.Generated
	}
}

/**
* Prepare experiment by merging with base config, creating output directory and writing experiment config to temp file
**/
func (d *MultiLoaderRunner) prepareExperiment(experiment types.LoaderExperiment) {
	log.Debug("Preparing ", experiment.Name)
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
**/
func (d *MultiLoaderRunner) mergeConfigurations(baseConfigPath string, experiment types.LoaderExperiment) config.LoaderConfiguration {
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

/**
* Write experiment configuration to a temp file for loader to read
**/
func (d *MultiLoaderRunner) writeExperimentConfigToTempFile(experimentConfig config.LoaderConfiguration, fileWritePath string) {
	experimentConfigBytes, _ := json.Marshal(experimentConfig)
	err := os.WriteFile(fileWritePath, experimentConfigBytes, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

/**
* Create log file & determine number of times to execute loader and run
**/
func (d *MultiLoaderRunner) runExperiment(experiment types.LoaderExperiment) error {
	log.Debug("Experiment configuration ", experiment.Config)

	// Create the log file
	logFilePath := path.Join(path.Dir(experiment.Config["OutputPathPrefix"].(string)), "loader.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	// Default number of tries
	numTries := 1
	// Should retry if not dry run and fail fast
	if !d.DryRun && !d.FailFast {
		numTries += NUM_OF_RETRIES
	}

	for i := 0; i < numTries; i++ {
		// Log retry attmempt if necessary
		if i != 0 {
			log.Info("Retrying experiment ", experiment.Name)
			logFile.WriteString("==================================RETRYING==================================\n")
		}
		// Run loader.go with experiment configs
		if err := d.executeLoaderCommand(experiment, logFile); err != nil {
			log.Error(err)
			log.Error("Experiment failed: ", experiment.Name)
			logFile.WriteString("Experiment failed: " + experiment.Name + ". Error: " + err.Error() + "\n")

			// Update experiment verbosity to debug
			experiment.Verbosity = "debug"
		} else {
			// Experiment succeeded
			log.Debug("Completed ", experiment.Name)
			return nil
		}
	}
	// Experiment failed
	d.DryRunSuccess = false
	log.Error("Check log file for more information: ", logFilePath)
	return err
}

/**
* Execute loader command and log outputs
**/
func (d *MultiLoaderRunner) executeLoaderCommand(experiment types.LoaderExperiment, logFile *os.File) error {
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

/**
* Helper to log loader std output
**/
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
				log.Info(strings.ReplaceAll(strings.ReplaceAll(message, "\\t", " "), "\\n", ""))
			}
		}
	}
}

/**
* Helper to log loader std error
**/
func (d *MultiLoaderRunner) logLoaderStdError(stdPipe io.ReadCloser, logFile *os.File) {
	scanner := bufio.NewScanner(stdPipe)
	for scanner.Scan() {
		m := scanner.Text()
		// write to log file
		logFile.WriteString(m + "\n")

		if m == "" {
			continue
		}

		// if its go downloading logs, output to debug
		if strings.Contains(m, "go: downloading") {
			log.Debug(m)
			continue
		}

		log.Error(m)
	}
}

/**
* Perform necessary cleanup after experiment
**/
func (d *MultiLoaderRunner) performCleanup() {
	log.Debug("Runnning Cleanup")
	// Remove temp file
	os.Remove(EXPERIMENT_TEMP_CONFIG_PATH)

	log.Debug("Cleanup completed")
}
