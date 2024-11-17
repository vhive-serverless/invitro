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
	NUM_OF_RETRIES              = 2
)

type MultiLoaderRunner struct {
	MultiLoaderConfig types.MultiLoaderConfiguration
	DryRunSuccess     bool
	Verbosity         string
	IatGeneration     bool
	Generated         bool
	DryRun            bool
	Platform          string
}

// init multi loader runner
func NewMultiLoaderRunner(configPath string, verbosity string, iatGeneration bool, generated bool) (*MultiLoaderRunner, error) {
	multiLoaderConfig := ml_common.ReadMultiLoaderConfigurationFile(configPath)

	// validate configuration
	ml_common.CheckMultiLoaderConfig(multiLoaderConfig)

	// determine platform
	platform := ml_common.DeterminePlatformFromConfig(multiLoaderConfig)

	runner := MultiLoaderRunner{
		MultiLoaderConfig: multiLoaderConfig,
		DryRunSuccess:     true,
		Verbosity:         verbosity,
		IatGeneration:     iatGeneration,
		Generated:         generated,
		DryRun:            false,
		Platform:          platform,
	}

	return &runner, nil
}

func (d *MultiLoaderRunner) RunDryRun() {
	log.Info("Running dry runs")
	d.DryRun = true
	d.run()
}

func (d *MultiLoaderRunner) RunActual() {
	log.Info("Running actual experiments")
	d.DryRun = false
	d.run()
}

func (d *MultiLoaderRunner) run() {
	// Run global prescript
	common.RunScript(d.MultiLoaderConfig.PreScript)
	// Iterate over studies and run them
	for _, study := range d.MultiLoaderConfig.Studies {
		log.Debug("Setting up study: ", study.Name)
		// Run pre script
		common.RunScript(study.PreScript)

		// Unpack study to a list of studies with different loader configs
		sparseExperiments := d.unpackStudy(study)

		// Iterate over sparse experiments, prepare and run
		for _, experiment := range sparseExperiments {

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
		if len(sparseExperiments) > 1 && !d.DryRun {
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
func (d *MultiLoaderRunner) unpackStudy(study types.LoaderStudy) []types.LoaderExperiment {
	log.Debug("Unpacking study ", study.Name)
	var experiments []types.LoaderExperiment

	// if user specified a trace directory
	if study.TracesDir != "" {
		experiments = d.unpackFromTraceDir(study)
		// user define trace format and values instead of directory
	} else if study.TracesFormat != "" && len(study.TraceValues) > 0 {
		experiments = d.unpackFromTraceValues(study)
	} else {
		// Theres only one experiment in the study
		experiments = d.unpackSingleExperiment(study)
	}

	return experiments
}

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
* Creates a LoaderExperiment form a given study and updates relevant expereiment fields
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
		experiment.IatGeneration = d.IatGeneration
	}
	if !experiment.Generated {
		experiment.Generated = d.Generated
	}
}

/**
* Prepare experiment by merging with base config, creating output directory and writing experiment config to temp file
 */
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
 */
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

func (d *MultiLoaderRunner) writeExperimentConfigToTempFile(experimentConfig config.LoaderConfiguration, fileWritePath string) {
	experimentConfigBytes, _ := json.Marshal(experimentConfig)
	err := os.WriteFile(fileWritePath, experimentConfigBytes, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func (d *MultiLoaderRunner) runExperiment(experiment types.LoaderExperiment) error {
	if d.DryRun {
		log.Info("Dry running ", experiment.Name)
	} else {
		log.Info("Running ", experiment.Name)
	}
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
			} else {
				// Experiment failed set dry run flag to false
				d.DryRunSuccess = false
				log.Error("Check log file for more information: ", logFilePath)
				// should not continue with experiment
				return err
			}
			continue
		} else {
			break
		}
	}
	log.Debug("Completed ", experiment.Name)
	return nil
}

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

func (d *MultiLoaderRunner) performCleanup() {
	log.Debug("Runnning Cleanup")
	// Run make clean
	if err := exec.Command("make", "clean").Run(); err != nil {
		log.Error("Error occured while running cleanup", err)
	}
	// Remove temp file
	os.Remove(EXPERIMENT_TEMP_CONFIG_PATH)

	log.Debug("Cleanup completed")
}
