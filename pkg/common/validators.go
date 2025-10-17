package common

import (
	"net"
	"os"
	"slices"

	log "github.com/sirupsen/logrus"
)

func CheckPath(path string) {
	if path == "" {
		return
	}
	_, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
	}
}

func IsValidIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil
}

func CheckCPULimit(cpuLimit string) {
	if !slices.Contains(ValidCPULimits, cpuLimit) {
		log.Fatal("Invalid CPU Limit ", cpuLimit)
	}
}
