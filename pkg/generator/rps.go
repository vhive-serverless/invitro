package generator

import (
	"github.com/vhive-serverless/loader/pkg/common"
	"math"
)

func generateFunctionByRPS(experimentDuration int, rpsTarget float64) (common.IATArray, []int) {
	iat := 1000000.0 / float64(rpsTarget) // ms

	var iatResult []float64
	var countResult []int

	duration := 0.0 // ms
	totalExperimentDurationMs := float64(experimentDuration * 60_000_000.0)

	currentMinute := 0
	currentCount := 0

	// beginning of each minute should have 0.0 -- see Leonid's changes
	iatResult = append(iatResult, 0.0)

	for duration < totalExperimentDurationMs {
		iatResult = append(iatResult, iat)
		duration += iat
		currentCount++

		if int(duration)/60_000_000 != currentMinute {
			countResult = append(countResult, currentCount)
			// beginning of each minute should have 0.0 -- see Leonid's changes
			iatResult = append(iatResult, 0.0)

			currentMinute++
			currentCount = 0
		}
	}

	return iatResult, countResult
}

func generateFunctionByRPSWithOffset(experimentDuration int, rpsTarget float64, offset float64) (common.IATArray, []int) {
	var resIAT common.IATArray
	var resCount []int

	iat, count := generateFunctionByRPS(experimentDuration, rpsTarget)

	if offset != 0 {
		resIAT = append(resIAT, -offset)
		count[0]++
	}

	resIAT = append(resIAT, iat...)
	resCount = append(resCount, count...)

	if iat == nil {
		resCount = append(resCount, 0)
	}

	return resIAT, resCount
}

func GenerateWarmStartFunction(experimentDuration int, rpsTarget float64) (common.IATArray, []int) {
	return generateFunctionByRPS(experimentDuration, rpsTarget)
}

func GenerateColdStartFunctions(experimentDuration int, rpsTarget float64, cooldownSeconds int) ([]common.IATArray, [][]int) {
	iat := 1000.0 / float64(rpsTarget) // ms
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
