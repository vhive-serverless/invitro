package utils

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func ReadSetupCfg(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	config := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.Trim(strings.TrimSpace(parts[1]), "'")
			config[key] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return config, nil
}

func ServerExec(node, command string) (string, error) {
	cmd := exec.Command("ssh", "-oStrictHostKeyChecking=no", "-p", "22", node, command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to execute command on node %s: %v, output: %s", node, err, string(output))
	}
	return string(output), nil
}
