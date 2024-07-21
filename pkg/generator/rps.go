package generator

import (
	"fmt"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"math"
	"math/rand"
)

func generateFunctionByRPS(experimentDuration int, rpsTarget float64) (common.IATArray, []int) {
	iat := 1000000.0 / float64(rpsTarget) // μs

	var iatResult []float64
	var countResult []int

	duration := 0.0 // μs
	totalExperimentDurationMs := float64(experimentDuration * 60_000_000.0)

	currentMinute := 0
	currentCount := 0

	for duration < totalExperimentDurationMs {
		iatResult = append(iatResult, iat)
		duration += iat
		currentCount++

		// count the number of invocations in minute
		if int(duration)/60_000_000 != currentMinute {
			countResult = append(countResult, currentCount)

			currentMinute++
			currentCount = 0
		}
	}

	return iatResult, countResult
}

func generateFunctionByRPSWithOffset(experimentDuration int, rpsTarget float64, offset float64) (common.IATArray, []int) {
	iat, count := generateFunctionByRPS(experimentDuration, rpsTarget)
	iat[0] += offset

	return iat, count
}

func GenerateWarmStartFunction(experimentDuration int, rpsTarget float64) (common.IATArray, []int) {
	if rpsTarget == 0 {
		return nil, nil
	}

	return generateFunctionByRPS(experimentDuration, rpsTarget)
}

func GenerateColdStartFunctions(experimentDuration int, rpsTarget float64, cooldownSeconds int) ([]common.IATArray, [][]int) {
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
			fx, count = generateFunctionByRPSWithOffset(experimentDuration, 1/float64(cooldownSeconds), float64(offset))
		} else {
			fx, count = generateFunctionByRPSWithOffset(experimentDuration, 1/(float64(totalFunctions)/rpsTarget), float64(offset))
		}

		functions = append(functions, fx)
		countResult = append(countResult, count)
	}

	return functions, countResult
}

func CreateRPSFunctions(cfg *config.LoaderConfiguration, warmFunction common.IATArray, warmFunctionCount []int,
	coldFunctions []common.IATArray, coldFunctionCount [][]int) []*common.Function {
	var result []*common.Function

	if warmFunction != nil || warmFunctionCount != nil {
		result = append(result, &common.Function{
			Name: fmt.Sprintf("warm_function_%d", rand.Int()),

			InvocationStats: &common.FunctionInvocationStats{Invocations: warmFunctionCount},
			MemoryStats:     &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
			DirigentMetadata: &common.DirigentMetadata{
				Image:               cfg.RpsImage,
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1024,
				ScalingLowerBound:   1,
				IterationMultiplier: cfg.RpsIterationMultiplier,
			},

			Specification: &common.FunctionSpecification{
				IAT:                  warmFunction,
				PerMinuteCount:       warmFunctionCount,
				RuntimeSpecification: createRuntimeSpecification(len(warmFunction), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},
		})
	}

	for i := 0; i < len(coldFunctions); i++ {
		result = append(result, &common.Function{
			Name: fmt.Sprintf("cold-function-%d-%d", i, rand.Int()),

			InvocationStats: &common.FunctionInvocationStats{Invocations: coldFunctionCount[i]},
			MemoryStats:     &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
			DirigentMetadata: &common.DirigentMetadata{
				Image:               cfg.RpsImage,
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1,
				ScalingLowerBound:   0,
				IterationMultiplier: cfg.RpsIterationMultiplier,
			},

			Specification: &common.FunctionSpecification{
				IAT:                  coldFunctions[i],
				PerMinuteCount:       coldFunctionCount[i],
				RuntimeSpecification: createRuntimeSpecification(len(coldFunctions[i]), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},
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
