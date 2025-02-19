package common

import (
	"bytes"
	"net"
	"os/exec"
	"path"
	"slices"
	"strings"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

// Check general multi-loader configuration that applies to all platforms
func CheckMultiLoaderConfig(multiLoaderConfig types.MultiLoaderConfiguration) {
	log.Debug("Checking multi-loader configuration")
	// Check if all paths are valid
	common.CheckPath(multiLoaderConfig.BaseConfigPath)
	// Check each study
	if len(multiLoaderConfig.Studies) == 0 {
		log.Fatal("No study found in configuration file")
	}
	platform := DeterminePlatformFromConfig(multiLoaderConfig)
	if platform == "" {
		log.Fatal("Platform not found in base configuration")
	}

	for _, study := range multiLoaderConfig.Studies {
		// Check if platform is defined, if so check if consistent with base config
		if _, ok := study.Config["Platform"]; ok {
			if study.Config["Platform"] != platform {
				log.Fatal("Platform in study ", study.Name, " is inconsistent with base configuration's platform ", platform)
			}
		}
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
	log.Debug("All experiments configs are valid")
}

func CheckKnativeSpecificMultiLoaderConfig(multiLoaderConfig types.MultiLoaderConfiguration) {
	log.Debug("Checking platform specific multi-loader configuration")
	// Check knative specific configurations
	// Check if metrics are valid
	for _, metric := range multiLoaderConfig.Metrics {
		CheckCollectableMetrics(metric)
	}
	// Check nodes
	CheckNode(multiLoaderConfig.MasterNode)
	CheckNode(multiLoaderConfig.AutoScalerNode)
	CheckNode(multiLoaderConfig.ActivatorNode)
	CheckNode(multiLoaderConfig.LoaderNode)
	for _, node := range multiLoaderConfig.WorkerNodes {
		CheckNode(node)
	}
	log.Debug("Nodes are reachable")
}

func CheckCollectableMetrics(metrics string) {
	if !slices.Contains(ValidCollectableMetrics, metrics) {
		log.Fatal("Invalid metrics ", metrics)
	}
}

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

func IsValidIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil
}
