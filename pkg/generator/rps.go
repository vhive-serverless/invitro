package generator

import (
	"fmt"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/pkg/config"
	"math"
	"math/rand"
)

func generateFunctionByRPS(experimentDuration int, rpsTarget float64) common.IATArray {
	iat := 1000000.0 / float64(rpsTarget) // μs

	duration := 0.0 // μs
	totalExperimentDurationMs := float64(experimentDuration * 60_000_000.0)

	var iatResult []float64
	for duration < totalExperimentDurationMs {
		iatResult = append(iatResult, iat)
		duration += iat
	}

	// make the first invocation be fired right away
	if len(iatResult) > 0 {
		iatResult[0] = 0
	}

	return iatResult
}

func countNumberOfInvocationsPerMinute(experimentDuration int, iatResult []float64) []int {
	result := make([]int, experimentDuration)

	cnt := make(map[int]int)
	timestamp := 0.0
	for i := 0; i < len(iatResult); i++ {
		t := timestamp + iatResult[i]
		minute := int(t) / 60_000_000
		cnt[minute]++
		timestamp = t
	}

	for i := 0; i < len(result); i++ {
		result[i] = cnt[i]
	}

	return result
}

func generateFunctionByRPSWithOffset(experimentDuration int, rpsTarget float64, offset float64) (common.IATArray, []int) {
	iat := generateFunctionByRPS(experimentDuration, rpsTarget)
	iat[0] += offset

	count := countNumberOfInvocationsPerMinute(experimentDuration, iat)
	return iat, count
}

func GenerateWarmStartFunction(experimentDuration int, rpsTarget float64) (common.IATArray, []int) {
	if rpsTarget == 0 {
		return nil, nil
	}

	iat := generateFunctionByRPS(experimentDuration, rpsTarget)
	count := countNumberOfInvocationsPerMinute(experimentDuration, iat)
	return iat, count
}

// GenerateColdStartFunctions Because Knative's minimum autoscaling stable window is 6s, the minimum keep-alive for a
// function is 6s. This means that we need multiple functions to achieve RPS=1, each scaling up/and down with a 1-second
// delay from each other. In RPS mode, the number of functions for the cold start experiment is determined by the
// RpsCooldownSeconds parameter, which is the minimum keep-alive. This produces the IAT array as above. Due to the
// implementation complexity, the cold start experiment sleeps for the first RpsCooldownSeconds seconds. In the results,
// the user should discard the first and the last RpsCooldownSeconds of the results, since the RPS at those points is
// lower than the requested one.
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

	busyLoopFor := ComputeBusyLoopPeriod(cfg.RpsMemoryMB)

	if warmFunction != nil || warmFunctionCount != nil {
		result = append(result, &common.Function{
			Name: fmt.Sprintf("warm-function-%d", rand.Int()),

			InvocationStats: &common.FunctionInvocationStats{Invocations: warmFunctionCount},
			MemoryStats:     &common.FunctionMemoryStats{Percentile100: float64(cfg.RpsMemoryMB)},
			DirigentMetadata: &common.DirigentMetadata{
				Image:               cfg.RpsImage,
				Port:                80,
				Protocol:            "tcp",
				ScalingUpperBound:   1024,
				ScalingLowerBound:   1,
				IterationMultiplier: cfg.RpsIterationMultiplier,
				IOPercentage:        0,
			},

			Specification: &common.FunctionSpecification{
				IAT:                  warmFunction,
				PerMinuteCount:       warmFunctionCount,
				RuntimeSpecification: createRuntimeSpecification(len(warmFunction), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},

			ColdStartBusyLoopMs: busyLoopFor,
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
				IOPercentage:        0,
			},

			Specification: &common.FunctionSpecification{
				IAT:                  coldFunctions[i],
				PerMinuteCount:       coldFunctionCount[i],
				RuntimeSpecification: createRuntimeSpecification(len(coldFunctions[i]), cfg.RpsRuntimeMs, cfg.RpsMemoryMB),
			},

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
