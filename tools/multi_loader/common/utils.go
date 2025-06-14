package common

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/vhive-serverless/loader/pkg/config"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

func ReadMultiLoaderConfigurationFile(path string) types.MultiLoaderConfiguration {
	byteValue, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	var config types.MultiLoaderConfiguration
	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		log.Fatal(err)
	}

	return config
}

func WriteMultiLoaderConfigurationFile(config types.MultiLoaderConfiguration, path string) {
	configByteValue, err := json.Marshal(config)
	if err != nil {
		log.Fatal(err)
	}

	err = os.WriteFile(path, configByteValue, 0644)
	if err != nil {
		log.Fatal(err)
	}
}

func DeterminePlatformFromConfig(multiLoaderConfig types.MultiLoaderConfiguration) string {
	// Determine platform
	baseConfigByteValue, err := os.ReadFile(multiLoaderConfig.BaseConfigPath)
	if err != nil {
		log.Fatal(err)
	}
	var loaderConfig config.LoaderConfiguration
	// Unmarshal base configuration
	if err = json.Unmarshal(baseConfigByteValue, &loaderConfig); err != nil {
		log.Fatal(err)
	}
	// transform platform to lowercase to match defined constant case
	return strings.ToLower(loaderConfig.Platform)
}

/**
 * NextCProduct generates the next Cartesian product of the given limits
 **/
func NextCProduct(limits []int) func() []int {
	permutations := make([]int, len(limits))
	indices := make([]int, len(limits))
	done := false

	return func() []int {
		// Check if there are more permutations
		if done {
			return nil
		}

		// Generate the current permutation
		copy(permutations, indices)

		// Generate the next permutation
		for i := len(indices) - 1; i >= 0; i-- {
			indices[i]++
			if indices[i] <= limits[i] {
				break
			}
			indices[i] = 0
			if i == 0 {
				// All permutations have been generated
				done = true
			}
		}

		return permutations
	}
}

func SplitPath(path string) []string {
	dir, last := filepath.Split(path)
	if dir == "" {
		return []string{last}
	}
	return append(SplitPath(filepath.Clean(dir)), last)
}

func SweepOptionsToPostfix(sweepOptions []types.SweepOptions, selectedSweepValues []int) string {
	var postfix string
	for i, sweepOption := range sweepOptions {
		postfix += fmt.Sprintf("_%s_%v", sweepOption.Field, sweepOption.Values[selectedSweepValues[i]])
	}
	return postfix
}

func UpdateExperimentWithSweepIndices(experiment *types.LoaderExperiment, sweepOptions []types.SweepOptions, selectedSweepValues []int) {
	experimentPostFix := SweepOptionsToPostfix(sweepOptions, selectedSweepValues)

	experiment.Name = experiment.Name + experimentPostFix
	paths := SplitPath(experiment.Config["OutputPathPrefix"].(string))
	// update the last two paths with the sweep indices
	paths[len(paths)-2] = paths[len(paths)-2] + experimentPostFix

	experiment.Config["OutputPathPrefix"] = path.Join(paths...)

	for sweepOptionI, sweepValueI := range selectedSweepValues {
		sweepValue := sweepOptions[sweepOptionI].GetValue(sweepValueI)
		if sweepOptions[sweepOptionI].Field == "PreScript" {
			experiment.PreScript = sweepValue.(string)
		} else if sweepOptions[sweepOptionI].Field == "PostScript" {
			experiment.PostScript = sweepValue.(string)
		} else {
			experiment.Config[sweepOptions[sweepOptionI].Field] = sweepValue
		}
	}
}

func DetermineWorkerNodeIPs() []string {
	out := DetermineNodeIP(Worker)
	workerNodes := strings.Split(out, "\n")
	for i := range workerNodes {
		workerNodes[i] = strings.TrimSpace(workerNodes[i])
	}
	return workerNodes
}

func DetermineNodeIP(node NodeType) string {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("kubectl get nodes --show-labels --no-headers -o wide | grep nodetype=%s | awk '{print $6}'", node))
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	return strings.Trim(string(out), " \n")
}

func DeterminePodIP(podNamePrefix PodType) string {
	// Get the pod alias
	cmdPodName := exec.Command("sh", "-c", fmt.Sprintf("kubectl get pods -n knative-serving --no-headers | grep %s- | awk '{print $1}'", podNamePrefix))
	out, err := cmdPodName.CombinedOutput()

	if err != nil {
		log.Fatal("Error getting", podNamePrefix, "pod name:", err)
	}

	// Get the private ip using the pod alias
	podName := strings.Trim(string(out), "\n")
	cmdNodeIP := exec.Command("sh", "-c", fmt.Sprintf("kubectl get pod %s -n knative-serving -o=jsonpath='{.status.hostIP}'", podName))
	out, err = cmdNodeIP.CombinedOutput()

	if err != nil {
		log.Fatal("Error getting", cmdNodeIP, "node IP:", err)
	}

	nodeIp := strings.Split(string(out), "\n")[0]
	return strings.Trim(nodeIp, " ")
}

func DetermineNodesIPs(multiLoaderConfig *types.MultiLoaderConfiguration) {
	log.Debug("Determining node IPs")

	var masterIP, loaderIP string
	var workerIPs []string

	switch {
	case IsKinD():
		nodeIP := DetermineNodeIP(Worker)
		masterIP = nodeIP
		loaderIP = nodeIP
		workerIPs = []string{nodeIP}
	case IsSingleNode():
		nodeIP := DetermineNodeIP(SingleNode)
		masterIP = nodeIP
		loaderIP = nodeIP
		workerIPs = []string{nodeIP}
	default:
		masterIP = DetermineNodeIP(Master)
		loaderIP = DetermineNodeIP(Loader)
		workerIPs = DetermineWorkerNodeIPs()
	}
	assignDefaults(&multiLoaderConfig.MasterNode, masterIP)
	assignDefaults(&multiLoaderConfig.LoaderNode, loaderIP)
	assignDefaultsSlice(&multiLoaderConfig.WorkerNodes, workerIPs)

	assignDefaults(&multiLoaderConfig.AutoScalerNode, DeterminePodIP(AutoScalerPod))
	assignDefaults(&multiLoaderConfig.ActivatorNode, DeterminePodIP(ActivatorPod))

	log.Trace("Node IPs determined", multiLoaderConfig)
}

/**
* Helper function to get unique node list
**/
func GetUniqueNodeList() []string {
	cmd := exec.Command("sh", "-c", "kubectl get nodes --show-labels --no-headers -o wide | awk '{print $6}'")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	nodes := strings.Fields(string(out))
	log.Debug("Unique Node List: ", nodes)
	return nodes
}

func IsKinD() bool {
	cmd := exec.Command("sh", "-c", "kind get clusters")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "knative"
}

func IsSingleNode() bool {
	cmd := exec.Command("sh", "-c", "kubectl get nodes --show-labels --no-headers | grep nodetype=singlenode | wc -l")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "1"
}

// Helper functions	for assigning default values
func assignDefaults(target *string, value string) {
	if *target == "" {
		*target = value
	}
}

func assignDefaultsSlice(target *[]string, value []string) {
	if len(*target) == 0 {
		*target = value
	}
}

func RunRemoteCommand(node string, command string) (string, error) {
	cmd := exec.Command("ssh", "-oStrictHostKeyChecking=no", "-p 22", node, command)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	if len(output) > 0 {
		log.Debug(node, string(output))
	}
	return string(output), nil
}

func CopyRemoteFile(remoteNode, src string, dest string) error {
	cmd := exec.Command("scp", "-rp", remoteNode+":"+src, dest)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	if len(out) > 0 {
		log.Debug(string(out))
	}
	return nil
}

func DecompressGZFile(filePath string) (string, error) {
	if !strings.HasSuffix(filePath, ".gz") {
		return "", errors.New("incorrect file type received")
	}
	newFileName := strings.TrimSuffix(filePath, ".gz")

	// Open compressed file
	gzFile, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open gzip file: %w", err)
	}
	defer gzFile.Close()

	// Create gzip reader
	gzReader, err := gzip.NewReader(gzFile)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	// Create output file
	outFile, err := os.Create(newFileName)
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Decompress and write to output file
	if _, err = io.Copy(outFile, gzReader); err != nil {
		return "", fmt.Errorf("failed to decompress data: %w", err)
	}

	// Remove the original compressed file
	if err := os.Remove(filePath); err != nil {
		return "", fmt.Errorf("failed to remove compressed file: %w", err)
	}

	return newFileName, nil
}
