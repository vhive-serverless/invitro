package generator

import (
	"github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"math"
)

func generateFunctionByRPS(experimentDuration int, rpsTarget float64) common.IATArray {
	var result []float64
	iat := 1000.0 / float64(rpsTarget) // ms

	duration := 0.0
	totalExperimentDurationMs := float64(experimentDuration * 60_000.0)

	for duration < totalExperimentDurationMs {
		result = append(result, iat)
		duration += iat
	}

	return result
}

func generateFunctionByRPSWithOffset(experimentDuration int, rpsTarget float64, offset float64) common.IATArray {
	var res common.IATArray

	if offset != 0 {
		res = append(res, -offset)
	}
	res = append(res, generateFunctionByRPS(experimentDuration, rpsTarget)...)

	return res
}

func generateWarmStartFunction(experimentDuration int, rpsTarget float64) common.IATArray {
	return generateFunctionByRPS(experimentDuration, rpsTarget)
}

func generateColdStartFunctions(experimentDuration int, rpsTarget float64, cooldownSeconds int) []common.IATArray {
	iat := 1000.0 / float64(rpsTarget) // ms

	var functions []common.IATArray
	totalFunctions := int(math.Ceil(rpsTarget * float64(cooldownSeconds)))

	for i := 0; i < totalFunctions; i++ {
		offsetWithinBatch := 0
		if rpsTarget >= 1 {
			offsetWithinBatch = int(float64(i%int(rpsTarget)) * iat)
		}

		offsetBetweenFunctions := int(float64(i)/rpsTarget) * 1_000
		offset := offsetWithinBatch + offsetBetweenFunctions

		logrus.Debugf("batch offset: %3.d; function offset: %d\n", offsetWithinBatch, offsetBetweenFunctions)

		var fx common.IATArray
		if rpsTarget >= 1 {
			fx = generateFunctionByRPSWithOffset(experimentDuration, 1/float64(cooldownSeconds), float64(offset))
		} else {
			fx = generateFunctionByRPSWithOffset(experimentDuration, 1/(float64(totalFunctions)/rpsTarget), float64(offset))
		}

		functions = append(functions, fx)
	}

	return functions
}
