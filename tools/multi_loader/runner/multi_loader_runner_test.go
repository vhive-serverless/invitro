package runner

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	ml_common "github.com/vhive-serverless/loader/tools/multi_loader/common"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

var (
	multiLoaderTestConfigPath string
	configPath                string
	rootPath                  string
)

func init() {
	wd, _ := os.Getwd()
	rootPath = path.Join(wd, "..", "..", "..")
}

func TestUnpackExperiment(t *testing.T) {
	cleanup, multiLoader := setup()
	defer cleanup()
	t.Run("Unpack using TracesDir (Success)", func(t *testing.T) {
		// Set TracesDir to test directory
		multiLoader.MultiLoaderConfig.Studies[0].TracesDir = path.Join(rootPath, "data", "multi_traces")

		for _, experiment := range multiLoader.MultiLoaderConfig.Studies {
			subExperiments := multiLoader.unpackStudy(experiment)
			expectedNames := []string{"test-experiment_example_1_test", "test-experiment_example_2_test", "test-experiment_example_3.1_test"}
			expectedOutputPrefixes := []string{"example_1_test", "example_2_test", "example_3.1_test"}
			validateUnpackedExperiment(t, subExperiments, experiment, expectedNames, expectedOutputPrefixes)
		}
	})

	t.Run("Unpack using TracesDir (Failure: Incorrect Dir)", func(t *testing.T) {
		expectFatal(t, func() {
			multiLoader.MultiLoaderConfig.Studies[0].TracesDir = "../test_data_incorrect"
			for _, experiment := range multiLoader.MultiLoaderConfig.Studies {
				_ = multiLoader.unpackStudy(experiment)
			}
		})
	})

	t.Run("Unpack using TraceFormat and TraceValues", func(t *testing.T) {
		multiLoader.MultiLoaderConfig.Studies[0].TracesDir = ""

		for _, experiment := range multiLoader.MultiLoaderConfig.Studies {
			subExperiments := multiLoader.unpackStudy(experiment)
			expectedNames := make([]string, len(experiment.TraceValues))
			for i, traceValue := range experiment.TraceValues {
				expectedNames[i] = experiment.Name + "_example_" + fmt.Sprintf("%v", traceValue) + "_test"
			}
			validateUnpackedExperiment(t, subExperiments, experiment, expectedNames, nil)
		}
	})

	t.Run("Unpack using tracePath", func(t *testing.T) {
		multiLoader.MultiLoaderConfig.Studies[0].TracesDir = ""
		multiLoader.MultiLoaderConfig.Studies[0].TracesFormat = ""
		multiLoader.MultiLoaderConfig.Studies[0].TraceValues = nil

		for _, experiment := range multiLoader.MultiLoaderConfig.Studies {
			subExperiments := multiLoader.unpackStudy(experiment)
			expectedNames := []string{experiment.Name + "_" + experiment.Name}
			validateUnpackedExperiment(t, subExperiments, experiment, expectedNames, nil)
		}
	})
}

func TestPrepareExperiment(t *testing.T) {
	cleanup, multiLoader := setup()
	defer cleanup()
	subExperiment := types.LoaderExperiment{
		Name: "example_1",
		Config: map[string]interface{}{
			"ExperimentDuration": 10,
			"TracePath":          path.Join(rootPath, "data", "multi_traces", "example_1_test"),
			"OutputPathPrefix":   "./test_output/example_1_test",
		},
	}

	if err := os.MkdirAll(filepath.Dir(EXPERIMENT_TEMP_CONFIG_PATH), 0755); err != nil {
		t.Fatalf("Failed to create temp config directory: %v", err)
	}
	multiLoader.prepareExperiment(subExperiment)

	// Check that the output directory and config file were created
	outputDir := "./test_output"
	tempConfigPath := EXPERIMENT_TEMP_CONFIG_PATH

	// Verify the output directory exists
	if _, err := os.Stat(outputDir); os.IsNotExist(err) {
		t.Errorf("Expected output directory '%s' to be created, but it was not", outputDir)
	}

	// Verify the temporary config file exists
	if _, err := os.Stat(tempConfigPath); os.IsNotExist(err) {
		t.Errorf("Expected temp config file '%s' to be created, but it was not", tempConfigPath)
	}

	// Clean up created files and directories
	os.RemoveAll("./tools")
	os.RemoveAll(outputDir)
}

// Test mergeConfigurations method
func TestMergeConfig(t *testing.T) {
	cleanup, multiLoader := setup()
	defer cleanup()

	newTracePath := path.Join(rootPath, "data", "multi_traces", "example_1_test")
	experiment := types.LoaderExperiment{
		Name: "example_1",
		Config: map[string]interface{}{
			"ExperimentDuration": 10,
			"TracePath":          newTracePath,
			"OutputPathPrefix":   "./test_output/example_1_test",
		},
	}
	outputConfig := multiLoader.mergeConfigurations(configPath, experiment)
	// Check if the configurations are merged
	if outputConfig.TracePath != newTracePath {
		t.Errorf("Expected TracePath to be '%v', got %v", newTracePath, experiment.Config["TracePath"])
	}
	if outputConfig.OutputPathPrefix != "./test_output/example_1_test" {
		t.Errorf("Expected OutputPathPrefix to be './test_output/example_1_test', got %v", experiment.Config["OutputPathPrefix"])
	}
	if outputConfig.ExperimentDuration != 10 {
		t.Errorf("Expected ExperimentDuration to be 10, got %v", experiment.Config["ExperimentDuration"])
	}
}

func TestMultiConfigValidator(t *testing.T) {
	cleanup, multiLoader := setup()
	defer cleanup()
	t.Run("CheckMultiLoaderConfig (Success)", func(t *testing.T) {
		// Check if all paths are valid
		ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
	})

	t.Run("CheckMultiLoaderConfig (Failure: No Study)", func(t *testing.T) {
		expectFatal(t, func() {
			temp := multiLoader.MultiLoaderConfig.Studies
			multiLoader.MultiLoaderConfig.Studies = nil
			ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
			multiLoader.MultiLoaderConfig.Studies = temp
		})
	})

	t.Run("CheckMultiLoaderConfig (Failure: Missing TracesDir, TracesFormat, TraceValues)", func(t *testing.T) {
		expectFatal(t, func() {
			multiLoader.MultiLoaderConfig.Studies[0].TracesDir = ""
			multiLoader.MultiLoaderConfig.Studies[0].TracesFormat = ""
			multiLoader.MultiLoaderConfig.Studies[0].TraceValues = nil
			ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
		})
	})

	t.Run("CheckMultiLoaderConfig (Failure: Invalid TracesFormat)", func(t *testing.T) {
		expectFatal(t, func() {
			multiLoader.MultiLoaderConfig.Studies[0].TracesFormat = "invalid_format"
			ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
		})
	})

	t.Run("CheckMultiLoaderConfig (Failure: Missing TracesValues)", func(t *testing.T) {
		expectFatal(t, func() {
			multiLoader.MultiLoaderConfig.Studies[0].TraceValues = nil
			multiLoader.MultiLoaderConfig.Studies[0].TracesDir = ""
			multiLoader.MultiLoaderConfig.Studies[0].TracesFormat = "example_{}_test"
			ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
		})
	})

	t.Run("CheckMultiLoaderConfig (Failure: Missing OutputDir)", func(t *testing.T) {
		expectFatal(t, func() {
			multiLoader.MultiLoaderConfig.Studies[0].Config["Platform"] = "Other Platform"
			ml_common.CheckMultiLoaderConfig(multiLoader.MultiLoaderConfig)
		})
	})
}

func setup() (func(), MultiLoaderRunner) {
	wd, _ := os.Getwd()
	multiLoaderTestConfigPath = filepath.Join(wd, "../multi_loader_config.json")
	configPath = filepath.Join(rootPath, "pkg/config/test_config.json")
	log.Info("Test config path: ", multiLoaderTestConfigPath)
	log.Info("Test config path: ", configPath)

	// Override the BaseConfigPath field in multi_loader_config.json
	mlConfig := ml_common.ReadMultiLoaderConfigurationFile(multiLoaderTestConfigPath)
	mlConfig.BaseConfigPath = filepath.Join(rootPath, "pkg/config/test_config.json")
	multiLoaderTestConfigPath = "../multi_loader_config_test.json"
	ml_common.WriteMultiLoaderConfigurationFile(mlConfig, multiLoaderTestConfigPath)

	// Create test_data
	filePath := filepath.Join(rootPath, "scripts", "setup", "setup_multi_test_trace.sh")
	cmd := exec.Command("bash", filePath)
	cmd.Dir = rootPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal("Failed to create test traces:", string(out), err)
	}

	cleanup := func() {
		os.Remove(multiLoaderTestConfigPath)
		os.RemoveAll(path.Join(rootPath, "data", "multi_traces"))
	}

	// Create a new multi-loader driver with the test config path
	multiLoader, err := NewMultiLoaderRunner(multiLoaderTestConfigPath, "info", false)
	if err != nil {
		log.Fatalf("Failed to create multi-loader driver: %v", err)
	}

	return cleanup, *multiLoader
}

// helper func to validate unpacked experiments
func validateUnpackedExperiment(t *testing.T, experimentConfig []types.LoaderExperiment, studyConfig types.LoaderStudy, expectedNames []string, expectedOutputPrefixes []string) {
	if len(experimentConfig) != len(expectedNames) {
		t.Errorf("Expected %d sub-experiments, got %d", len(expectedNames), len(experimentConfig))
		return
	}

	for i, subExp := range experimentConfig {
		// check name
		if subExp.Name != expectedNames[i] {
			t.Errorf("Expected subexperiment name '%s', got '%s'", expectedNames[i], subExp.Name)
		}

		// validate selected configs
		if subExp.Config["ExperimentDuration"] != studyConfig.Config["ExperimentDuration"] {
			t.Errorf("Expected ExperimentDuration %v, got %v", studyConfig.Config["ExperimentDuration"], subExp.Config["ExperimentDuration"])
		}
		if subExp.OutputDir != studyConfig.OutputDir {
			t.Errorf("Expected OutputDir '%s', got '%s'", studyConfig.OutputDir, subExp.OutputDir)
		}

		// check OutputPathPrefix if needed
		if len(expectedOutputPrefixes) > 0 {
			if outputPathPrefix, ok := subExp.Config["OutputPathPrefix"].(string); !(ok && strings.HasSuffix(outputPathPrefix, expectedOutputPrefixes[i])) {
				t.Errorf("Expected OutputPathPrefix '%s', got '%s'", expectedOutputPrefixes[i], subExp.Config["OutputPathPrefix"])
			}
		}
	}
}

func expectFatal(t *testing.T, funcToTest func()) {
	fatal := false
	originalExitFunc := log.StandardLogger().ExitFunc
	log.Info("Expecting a fatal message during the test, overriding the exit function")
	// Replace logrus exit function
	log.StandardLogger().ExitFunc = func(int) { fatal = true }

	funcToTest()
	// Restore original exit function
	log.StandardLogger().ExitFunc = originalExitFunc
	assert.True(t, fatal, "Expected log.Fatal to be called")
}
