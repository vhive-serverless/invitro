package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

func ReadMultiLoaderConfigurationFile(path string) types.MultiLoaderConfiguration {
	byteValue, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var config types.MultiLoaderConfiguration
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}

func WriteMultiLoaderConfigurationFile(config types.MultiLoaderConfiguration, path string) {
	configByteValue, err := json.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(path, configByteValue, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func DeterminePlatformFromConfig(multiLoaderConfig types.MultiLoaderConfiguration) string {
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

/**
 * NextCProduct generates the next Cartesian product of the given limits
 **/
func NextCProduct(limits []int) func() []int {
	permutations := make([]int, len(limits))
	indices := make([]int, len(limits))
	done := false

	return func() []int {
		// Check if there are more permutations
		if done {
			return nil
		}

		// Generate the current permutation
		copy(permutations, indices)

		// Generate the next permutation
		for i := len(indices) - 1; i >= 0; i-- {
			indices[i]++
			if indices[i] <= limits[i] {
				break
			}
			indices[i] = 0
			if i == 0 {
				// All permutations have been generated
				done = true
			}
		}

		return permutations
	}
}

func SplitPath(path string) []string {
	dir, last := filepath.Split(path)
	if dir == "" {
		return []string{last}
	}
	return append(SplitPath(filepath.Clean(dir)), last)
}

func SweepOptionsToPostfix(sweepOptions []types.SweepOptions, selectedSweepValues []int) string {
	var postfix string
	for i, sweepOption := range sweepOptions {
		postfix += fmt.Sprintf("_%s_%v", sweepOption.Field, sweepOption.Values[selectedSweepValues[i]])
	}
	return postfix
}

func UpdateExperimentWithSweepIndices(experiment *types.LoaderExperiment, sweepOptions []types.SweepOptions, selectedSweepValues []int) {
	experimentPostFix := SweepOptionsToPostfix(sweepOptions, selectedSweepValues)

	experiment.Name = experiment.Name + experimentPostFix
	paths := SplitPath(experiment.Config["OutputPathPrefix"].(string))
	// update the last two paths with the sweep indices
	paths[len(paths)-2] = paths[len(paths)-2] + experimentPostFix
	paths[len(paths)-1] = paths[len(paths)-1] + experimentPostFix

	experiment.Config["OutputPathPrefix"] = path.Join(paths...)

	for sweepOptionI, sweepValueI := range selectedSweepValues {
		sweepValue := sweepOptions[sweepOptionI].GetValue(sweepValueI)
		if sweepOptions[sweepOptionI].Field == "PreScript" {
			experiment.PreScript = sweepValue.(string)
		} else if sweepOptions[sweepOptionI].Field == "PostScript" {
			experiment.PostScript = sweepValue.(string)
		} else {
			experiment.Config[sweepOptions[sweepOptionI].Field] = sweepValue
		}
	}
}
