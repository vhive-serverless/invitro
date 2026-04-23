package utils

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func ResolveConfigDir(input string, configName string) (string, error) {
	if input != "" {
		if isValidConfigDir(input) {
			return input, nil
		}
		return "", fmt.Errorf("config directory %q does not contain setup.json and prometheus/prom_config.json", input)
	}

	candidates := []string{
		"configs",
		"scripts/setup/configs",
	}

	for _, dir := range candidates {
		if isValidConfigDir(dir) {
			return dir, nil
		}
	}

	return "", fmt.Errorf("could not locate config directory; pass -config-dir")
}

type tempNodeSetup struct {
	NodeSetup struct {
		MasterNode []string `json:"MASTER_NODE"`
		LoaderNode []string `json:"LOADER_NODE"`
		WorkerNode []string `json:"WORKER_NODE"`
	} `json:"NODE_SETUP"`
	NodeLabel map[string][]string `json:"NODE_LABEL"`
	NodeURL   []string            `json:"NODE_URL"`
}

func CreateTempNodeSetup(configDir string, nodes []string) (string, error) {
	if len(nodes) < 2 {
		return "", fmt.Errorf("expected at least <master_node@IP> <loader_node@IP>")
	}

	const tempConfigName = "node_setup_temp.json"

	setup := tempNodeSetup{
		NodeLabel: make(map[string][]string),
		NodeURL:   append([]string(nil), nodes...),
	}

	setup.NodeSetup.MasterNode = []string{"10.0.1.1"}
	setup.NodeSetup.LoaderNode = []string{"10.0.1.2"}

	workerNodes := make([]string, 0, len(nodes)-1)
	workerLabelNodes := make([]string, 0, len(nodes)-2)

	for i := 1; i < len(nodes); i++ {
		internalIP := fmt.Sprintf("10.0.1.%d", i+1)
		workerNodes = append(workerNodes, internalIP)
		if i > 1 {
			workerLabelNodes = append(workerLabelNodes, internalIP)
		}
	}

	setup.NodeSetup.WorkerNode = workerNodes
	setup.NodeLabel["loader-nodetype=master"] = []string{"10.0.1.1"}
	setup.NodeLabel["loader-nodetype=monitoring"] = []string{"10.0.1.2"}
	setup.NodeLabel["loader-nodetype=worker"] = workerLabelNodes

	configData, err := json.MarshalIndent(setup, "", "    ")
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(configDir, tempConfigName)
	if err := os.WriteFile(configPath, append(configData, '\n'), 0o644); err != nil {
		return "", err
	}

	return tempConfigName, nil
}

func isValidConfigDir(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, "setup.json")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "prometheus", "prom_config.json")); err != nil {
		return false
	}
	return true
}
