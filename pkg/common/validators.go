package common

import (
	"log"
	"slices"
)

func CheckCPULimit(cpuLimit string) {
	if !slices.Contains(ValidCPULimits, cpuLimit) {
		log.Fatal("Invalid CPU Limit ", cpuLimit)
	}
}