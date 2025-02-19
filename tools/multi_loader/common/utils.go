package common

import (
	"encoding/json"
	"fmt"
	"os"
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
		if sweepOption.Field == "PreScript" {
			postfix += fmt.Sprintf("_%s_%v", sweepOption.Field, selectedSweepValues[i])
		} else if sweepOption.Field == "PostScript" {
			postfix += fmt.Sprintf("_%s_%v", sweepOption.Field, selectedSweepValues[i])
		} else {
			postfix += fmt.Sprintf("_%s_%v", sweepOption.Field, sweepOption.Values[selectedSweepValues[i]])
		}
	}
	return postfix
}
