package common

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"

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
	if (path) == "" {
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

// Check general multi-loader configuration that applies to all platforms
func CheckMultiLoaderConfig(multiLoaderConfig MultiLoaderConfiguration) {
	log.Info("Checking multi-loader configuration")
	// Check if all paths are valid
	CheckPath(multiLoaderConfig.BaseConfigPath)
	// Check each study
	if len(multiLoaderConfig.Studies) == 0 {
		log.Fatal("No study found in configuration file")
	}
	for _, study := range multiLoaderConfig.Studies {
		// Check trace directory
		// if configs does not have TracePath or OutputPathPreix, either TracesDir or (TracesFormat and TraceValues) should be defined along with OutputDir
		if study.TracesDir == "" && (study.TracesFormat == "" || len(study.TraceValues) == 0) {
			if _, ok := study.Config["TracePath"]; !ok {
				log.Fatal("Missing one of TracesDir, TracesFormat & TraceValues, Config.TracePath in multi_loader_config ", study.Name)
			}
		}
		if study.TracesFormat != "" {
			// check if trace format contains TRACE_FORMAT_STRING
			if !strings.Contains(study.TracesFormat, TraceFormatString) {
				log.Fatal("Invalid TracesFormat in multi_loader_config ", study.Name, ". Missing ", TraceFormatString, " in format")
			}
		}
		if study.OutputDir == "" {
			if _, ok := study.Config["OutputPathPrefix"]; !ok {
				log.Warn("Missing one of OutputDir or Config.OutputPathPrefix in multi_loader_config ", study.Name)
				// set default output directory
				study.OutputDir = path.Join("data", "out", study.Name)
				log.Warn("Setting default output directory to ", study.OutputDir)
			}
		}
	}
	log.Info("All experiments configs are valid")
}

func CheckCollectableMetrics(metrics string) {
	if !slices.Contains(ValidCollectableMetrics, metrics) {
		log.Fatal("Invalid metrics ", metrics)
	}
}

func CheckCPULimit(cpuLimit string) {
	if !slices.Contains(ValidCPULimits, cpuLimit) {
		log.Fatal("Invalid CPU Limit ", cpuLimit)
	}
}
