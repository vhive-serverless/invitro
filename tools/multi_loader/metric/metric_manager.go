package metric

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	ml_common "github.com/vhive-serverless/loader/tools/multi_loader/common"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

const (
	TOP_FILENAME        = "top.txt"
	TOP_DIR_NAME        = "top"
	AUTOSCALER_DIR_NAME = "autoscaler"
	ACTIVATOR_DIR_NAME  = "activator"
	PROMETH_DIR_NAME    = "prometheus_snapshot"
)

type MetricManager struct {
	platform          string
	metricsToCollect  []string
	outputDir         string
	multiLoaderConfig types.MultiLoaderConfiguration
}

func NewMetricManager(platform string, multiLoaderConfig types.MultiLoaderConfiguration) *MetricManager {
	return &MetricManager{
		platform:          platform,
		metricsToCollect:  multiLoaderConfig.Metrics,
		multiLoaderConfig: multiLoaderConfig,
	}
}

/**
 * Collects metrics defined in metricsToCollect field. Does the necessary checks, dumps logs or metrics to the provided output directory
 */
func (m *MetricManager) CollectMetrics() {
	// Check if should collect metrics
	if len(m.metricsToCollect) == 0 {
		log.Debug("No metrics to collect")
		return
	}
	log.Debug("Collecting Metrics")

	if m.shouldCollect(ml_common.TOP) {
		m.collectTOPMetric()
	}

	if m.shouldCollect(ml_common.AutoScaler) {
		m.collectAutoScalerLogs()
	}

	if m.shouldCollect(ml_common.Activator) {
		m.collectActivatorLogs()
	}

	if m.shouldCollect(ml_common.Prometheus) {
		m.collectPrometheusSnapshot()
	}
}

/**
* Resets top processes in all nodes
**/
func (m *MetricManager) ResetTOP() {
	log.Debug("Resetting top process")
	// Check if should reset
	if !m.shouldCollect(ml_common.TOP) {
		return
	}
	// Reset top process
	var wg sync.WaitGroup
	for _, node := range m.getUniqueNodeList() {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			ml_common.RunRemoteCommand(node, "if pgrep top >/dev/null; then killall top; fi")
			ml_common.RunRemoteCommand(node, "top -b -d 15 -c -w 512 > top.txt 2>&1 &")
		}(node)
	}
	wg.Wait()
}

/**
* Clears collected metrics by deleting folders and files created during collection
**/
func (m *MetricManager) ClearCollectedMetrics() {
	log.Debug("Clearing collected metrics")
	if m.shouldCollect(ml_common.TOP) {
		os.RemoveAll(path.Join(m.outputDir, TOP_DIR_NAME))
	}
	if m.shouldCollect(ml_common.AutoScaler) {
		os.RemoveAll(path.Join(m.outputDir, AUTOSCALER_DIR_NAME))
	}
	if m.shouldCollect(ml_common.Activator) {
		os.RemoveAll(path.Join(m.outputDir, ACTIVATOR_DIR_NAME))
	}
	if m.shouldCollect(ml_common.Prometheus) {
		os.RemoveAll(path.Join(m.outputDir, PROMETH_DIR_NAME))
	}
}

/**
* Sets output directory
**/
func (m *MetricManager) SetOutputDir(outputDir string) {
	m.outputDir = outputDir
}

/**
* Collects top process metrics
**/
func (m *MetricManager) collectTOPMetric() {
	log.Debug("Collecting top metrics")
	// Collect top process metrics
	topDir := path.Join(m.outputDir, TOP_DIR_NAME)
	if err := os.MkdirAll(topDir, 0755); err != nil {
		log.Fatal(err)
	}
	var wg sync.WaitGroup
	for _, node := range m.getUniqueNodeList() {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			// kill all instances of top
			ml_common.RunRemoteCommand(node, "if pgrep top >/dev/null; then killall top; fi")
			// Copy top output to local
			ml_common.CopyRemoteFile(node, TOP_FILENAME, path.Join(topDir, "top_"+node+".txt"))
		}(node)
	}
	wg.Wait()
}

/**
* Collects autoscaler logs from autoscaler node
**/
func (m *MetricManager) collectAutoScalerLogs() {
	log.Debug("Collecting autoscaler logs from node: ", m.multiLoaderConfig.AutoScalerNode)
	autoScalerOutputDir := path.Join(m.outputDir, AUTOSCALER_DIR_NAME)
	err := os.MkdirAll(autoScalerOutputDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	// Retrieve autoscaler logs
	ml_common.CopyRemoteFile(m.multiLoaderConfig.AutoScalerNode, "/var/log/pods/knative-serving_autoscaler-*/autoscaler/*", autoScalerOutputDir)
}

/**
* Collects activator logs from activator node
**/
func (m *MetricManager) collectActivatorLogs() {
	log.Debug("Collecting activator logs from node: ", m.multiLoaderConfig.ActivatorNode)
	activatorOutputDir := path.Join(m.outputDir, ACTIVATOR_DIR_NAME)
	err := os.MkdirAll(activatorOutputDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	// Retrieve activator logs
	ml_common.CopyRemoteFile(m.multiLoaderConfig.ActivatorNode, "/var/log/pods/knative-serving_activator-*/activator/*", activatorOutputDir)
}

/**
* Collects prometheus snapshot from master node
**/
func (m *MetricManager) collectPrometheusSnapshot() {
	log.Debug("Collecting prometheus snapshot from node: ", m.multiLoaderConfig.MasterNode)
	// Ensure output dir exists
	promethOutputDir := path.Join(m.outputDir, PROMETH_DIR_NAME)
	err := os.MkdirAll(promethOutputDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	// Fetch prometheus snapshot with retries
	snapshot, err := m.fetchPrometheusSnapshot(10)
	// Handle failure to retrieve snapshot
	if err != nil {
		log.Error("Failed to retrieve Prometheus snapshot", err)
		return
	}
	// Check if snapshot status is successful
	if snapshot.Status != "success" {
		log.Error("Prometheus snapshot status not successful: ", snapshot)
		return
	}
	// Copy prometheus snapshot to file
	var tempSnapshotDir = "~/tmp/prometheus_snapshot"
	ml_common.RunRemoteCommand(m.multiLoaderConfig.MasterNode, "mkdir -p "+tempSnapshotDir)
	ml_common.RunRemoteCommand(m.multiLoaderConfig.MasterNode, "kubectl cp -n monitoring "+"prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/ "+
		"-c prometheus "+tempSnapshotDir)
	ml_common.CopyRemoteFile(m.multiLoaderConfig.MasterNode, tempSnapshotDir, path.Dir(promethOutputDir))
	// remove temp directory
	ml_common.RunRemoteCommand(m.multiLoaderConfig.MasterNode, "rm -rf "+tempSnapshotDir)
}

/**
* Fetches prometheus snapshot from master node with retries
* The first call to snapshot endpoint always fails, so theres a need for maxAttempts > 1
**/
func (m *MetricManager) fetchPrometheusSnapshot(maxAttempts int) (types.PrometheusSnapshot, error) {
	var snapshot types.PrometheusSnapshot
	snapshotCmd := "curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot"
	re := regexp.MustCompile(`\{.*\}`)

	for attempts := maxAttempts; attempts > 0; attempts-- {
		out, err := exec.Command("ssh", m.multiLoaderConfig.MasterNode, snapshotCmd).CombinedOutput()
		if err != nil {
			// Last attempt and still failed
			if attempts == 1 {
				return snapshot, fmt.Errorf("failed to retrieve prometheus snapshot: %v", err)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		// Unmarshal into snapshot struct
		jsonBytes := re.Find(out)
		if err = json.Unmarshal(jsonBytes, &snapshot); err != nil {
			return snapshot, fmt.Errorf("failed to unmarshal prometheus snapshot: %v", err)
		}
		// Directly return if snapshot status is successful
		if snapshot.Status == "success" {
			return snapshot, nil
		}
		log.Debug("Prometheus snapshot not ready. Retrying...")
	}
	return snapshot, fmt.Errorf("exhausted all attempts to retrieve Prometheus snapshot")
}

/**
* Helper function to get unique node list
**/
func (m *MetricManager) getUniqueNodeList() []string {
	cmd := exec.Command("sh", "-c", "kubectl get nodes --show-labels --no-headers -o wide | awk '{print $6}'")
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}
	nodes := strings.Fields(string(out))
	log.Debug("Unique Node List: ", nodes)
	return nodes
}

/**
 * Helper function to check if the metrics should be collected
 */
func (m *MetricManager) shouldCollect(targetMetrics string) bool {
	// Only collect for Knative
	if !strings.HasPrefix(m.platform, "Knative") {
		return false
	}
	for _, metric := range m.metricsToCollect {
		if metric == targetMetrics {
			return true
		}
	}
	return false
}
