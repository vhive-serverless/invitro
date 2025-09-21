package generator

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
)

func generateFunctionByRPS(functionIndex int, experimentDuration int, granularity common.TraceGranularity, rpsTarget float64, iatType common.IatDistribution, shiftIAT bool, seed int64) common.IATArray {
	var iatRand *rand.Rand
	if iatType == common.Exponential || iatType == common.Uniform {
		iatRand = rand.New(rand.NewSource(seed + int64(functionIndex)))
	}

	duration := 0.0 // μs

	totalExperimentDurationUs := float64(experimentDuration * common.OneSecondInMicroseconds)
	if granularity == common.MinuteGranularity {
		totalExperimentDurationUs = float64(experimentDuration * 60)
	}

	var iatResult []float64
	for {
		var iat float64
		if iatType == common.Equidistant {
			iat = 1000000.0 / float64(rpsTarget) // μs
		} else if iatType == common.Exponential {
			iat = iatRand.ExpFloat64() * 1000000.0 / float64(rpsTarget)
		} else if iatType == common.Uniform {
			iat = 2 * iatRand.Float64() * 1000000.0 / float64(rpsTarget)
		} else {
			logrus.Fatal("Unsupported IAT distribution for RPS mode.")
		}
		duration += iat
		if duration > totalExperimentDurationUs {
			break
		}
		iatResult = append(iatResult, iat)
	}

	// make the first invocation be fired right away
	if len(iatResult) > 0 {
		iatResult[0] = 0
	}

	return iatResult
}

func countNumberOfInvocationsPerMinute(experimentDuration int, granularity common.TraceGranularity, iatResult []float64) []int {
	result := make([]int, experimentDuration)

	// set zero count for each minute
	for i := 0; i < experimentDuration; i++ {
		result[i] = 0
	}

	cnt := make(map[int]int)
	timestamp := 0.0
	interval := 0
	if granularity == common.MinuteGranularity {
		interval = 60_000_000
	} else {
		interval = 1_000_000
	}
	for i := 0; i < len(iatResult); i++ {
		t := timestamp + iatResult[i]
		minute := int(t) / interval
		cnt[minute]++
		timestamp = t
	}

	// update minutes based on counts
	for key, value := range cnt {
		result[key] = value
	}

	return result
}

func generateFunctionByRPSWithOffset(functionIndex int, experimentDuration int, granularity common.TraceGranularity, rpsTarget float64, offset float64, iatType common.IatDistribution, shiftIAT bool, seed int64) (common.IATArray, []int) {
	iat := generateFunctionByRPS(functionIndex, experimentDuration, granularity, rpsTarget, iatType, shiftIAT, seed)
	iat[0] += offset

	count := countNumberOfInvocationsPerMinute(experimentDuration, granularity, iat)
	return iat, count
}

func GenerateWarmStartFunction(functionIndex int, experimentDuration int, granularity common.TraceGranularity, rpsTarget float64, iatType common.IatDistribution, shiftIAT bool, seed int64) (common.IATArray, []int) {
	if rpsTarget == 0 {
		return nil, countNumberOfInvocationsPerMinute(experimentDuration, granularity, nil)
	}

	iat := generateFunctionByRPS(functionIndex, experimentDuration, granularity, rpsTarget, iatType, shiftIAT, seed)
	count := countNumberOfInvocationsPerMinute(experimentDuration, granularity, iat)
	return iat, count
}

// GenerateColdStartFunctions It is recommended that the first 10% of cold starts are discarded from the experiment results for low cold start RPS.
func GenerateColdStartFunctions(experimentDuration int, granularity common.TraceGranularity, rpsTarget float64, cooldownSeconds int, iatType common.IatDistribution, shiftIAT bool, seed int64) ([]common.IATArray, [][]int) {
	iat := 1000000.0 / float64(rpsTarget) // ms
	totalFunctions := int(math.Ceil(rpsTarget * float64(cooldownSeconds)))

	var functions []common.IATArray
	var countResult [][]int

	for i := 0; i < totalFunctions; i++ {
		offsetWithinBatch := 0
		if rpsTarget >= 1 {
			offsetWithinBatch = int(float64(i%int(rpsTarget)) * iat)
		}

		offsetBetweenFunctions := int(float64(i)/rpsTarget) * 1_000_000
		offset := offsetWithinBatch + offsetBetweenFunctions

		var fx common.IATArray
		var count []int
		if rpsTarget >= 1 {
			fx, count = generateFunctionByRPSWithOffset(i, experimentDuration, granularity, 1/float64(cooldownSeconds), float64(offset), iatType, shiftIAT, seed)
		} else {
			fx, count = generateFunctionByRPSWithOffset(i, experimentDuration, granularity, 1/(float64(totalFunctions)/rpsTarget), float64(offset), iatType, shiftIAT, seed)
		}

		functions = append(functions, fx)
		countResult = append(countResult, count)
	}

	logrus.Warn("It is recommended that the first 10% of cold starts are discarded from the experiment results for low cold start RPS.")
	return functions, countResult
}

func CreateRPSFunctions(cfg *config.LoaderConfiguration, dcfg *config.DirigentConfig, warmFunction []common.IATArray, warmFunctionCount [][]int,
	coldFunctions []common.IATArray, coldFunctionCount [][]int, yamlPath string) []*common.Function {
	var result []*common.Function

	busyLoopFor := ComputeBusyLoopPeriod(cfg.RpsMemoryMB)

	if warmFunction != nil || warmFunctionCount != nil {
		var dirigentMetadataWarm *common.DirigentMetadata
		if dcfg != nil {
			dirigentMetadataWarm = &common.DirigentMetadata{
				Image:               dcfg.RpsImage,
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1024,
				ScalingLowerBound:   1,
				IterationMultiplier: cfg.RpsIterationMultiplier,
				IOPercentage:        0,
			}
		}

		for i := 0; i < len(warmFunction); i++ {
			result = append(result, &common.Function{
				Name: fmt.Sprintf("warm-function-%d-%d", i, rand.Int()),

				InvocationStats:  &common.FunctionInvocationStats{Invocations: warmFunctionCount[i]},
				RuntimeStats:     &common.FunctionRuntimeStats{Average: float64(cfg.RpsRuntimeMs)},
				MemoryStats:      &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
				DirigentMetadata: dirigentMetadataWarm,

				Specification: &common.FunctionSpecification{
					IAT:                  warmFunction[i],
					PerMinuteCount:       warmFunctionCount[i],
					RuntimeSpecification: createRuntimeSpecification(len(warmFunction[i]), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
				},

				YAMLPath:            yamlPath,
				ColdStartBusyLoopMs: busyLoopFor,
			})
		}
	}

	for i := 0; i < len(coldFunctions); i++ {
		var dirigentMetadataCold *common.DirigentMetadata
		if dcfg != nil {
			dirigentMetadataCold = &common.DirigentMetadata{
				Image:               dcfg.RpsImage,
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1,
				ScalingLowerBound:   0,
				IterationMultiplier: cfg.RpsIterationMultiplier,
				IOPercentage:        0,
			}
		}

		result = append(result, &common.Function{
			Name: fmt.Sprintf("cold-function-%d-%d", i, rand.Int()),

			InvocationStats:  &common.FunctionInvocationStats{Invocations: coldFunctionCount[i]},
			MemoryStats:      &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
			DirigentMetadata: dirigentMetadataCold,

			Specification: &common.FunctionSpecification{
				IAT:                  coldFunctions[i],
				PerMinuteCount:       coldFunctionCount[i],
				RuntimeSpecification: createRuntimeSpecification(len(coldFunctions[i]), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},

			YAMLPath:            yamlPath,
			ColdStartBusyLoopMs: busyLoopFor,
		})
	}

	return result
}

func createRuntimeSpecification(count int, runtime, memory int) common.RuntimeSpecificationArray {
	var result common.RuntimeSpecificationArray
	for i := 0; i < count; i++ {
		result = append(result, common.RuntimeSpecification{
			Runtime: runtime,
			Memory:  memory,
		})
	}

	return result
}
