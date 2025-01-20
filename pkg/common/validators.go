package common

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"slices"

	log "github.com/sirupsen/logrus"
)

func CheckNode(node string) {
	if !IsValidIP(node) {
		log.Fatal("Invalid IP address for node ", node)
	}
	cmd := exec.Command("ssh", "-oStrictHostKeyChecking=no", "-p", "22", node, "exit")
	// -oStrictHostKeyChecking=no -p 22
	out, err := cmd.CombinedOutput()
	if bytes.Contains(out, []byte("Permission denied")) || err != nil {
		log.Error(string(out))
		log.Fatal("Failed to connect to node ", node)
	}
}

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
