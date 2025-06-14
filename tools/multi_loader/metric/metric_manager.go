package metric

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/common"
	ml_common "github.com/vhive-serverless/loader/tools/multi_loader/common"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

const (
	TOP_FILENAME        = "top.txt"
	TOP_DIR_NAME        = "top"
	AUTOSCALER_DIR_NAME = "autoscaler"
	ACTIVATOR_DIR_NAME  = "activator"
	PROMETH_DIR_NAME    = "prometheus_snapshot"
	TIMESTAMP_FORMAT    = "20060102-150405"
)

type MetricManager struct {
	platform          string
	metricsToCollect  []string
	outputDir         string
	multiLoaderConfig types.MultiLoaderConfiguration
	startTime         time.Time
}

func NewMetricManager(platform string, multiLoaderConfig types.MultiLoaderConfiguration) *MetricManager {
	return &MetricManager{
		platform:          strings.ToLower(platform),
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
		m.collectPodLogs(m.multiLoaderConfig.AutoScalerNode, ml_common.AutoScalerPod.String(), AUTOSCALER_DIR_NAME)
	}

	if m.shouldCollect(ml_common.Activator) {
		m.collectPodLogs(m.multiLoaderConfig.ActivatorNode, ml_common.ActivatorPod.String(), ACTIVATOR_DIR_NAME)
	}

	if m.shouldCollect(ml_common.Prometheus) {
		m.collectPrometheusSnapshot()
	}
}

/**
* Reset the start time from which log/metrics should be collected and TOP for all nodes
**/
func (m *MetricManager) Reset() {
	// Reset TOP for all nodes
	m.resetTOP()
	// Set the start time for logs collected
	m.startTime = time.Now()
}

/**
* Resets top processes in all nodesa
**/
func (m *MetricManager) resetTOP() {
	log.Debug("Resetting top process")
	// Check if should reset
	if !m.shouldCollect(ml_common.TOP) {
		return
	}
	// Reset top process
	var wg sync.WaitGroup
	for _, node := range ml_common.GetUniqueNodeList() {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			_, err := ml_common.RunRemoteCommand(node, "if pgrep top >/dev/null; then killall top; fi")
			if err != nil {
				log.Fatal("Failed to killall TOP for node: ", node, err)
			}
			_, err = ml_common.RunRemoteCommand(node, "top -b -d 15 -c -w 512 > top.txt 2>&1 &")
			if err != nil {
				log.Fatal("Failed to dump TOP info into temp txt file for node: ", node, err)
			}

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
	for _, node := range ml_common.GetUniqueNodeList() {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			// kill all instances of top
			_, err := ml_common.RunRemoteCommand(node, "if pgrep top >/dev/null; then killall top; fi")
			if err != nil {
				log.Fatal("Failed to killall TOP for node: ", node, err)
			}
			// Copy top output to local
			err = ml_common.CopyRemoteFile(node, TOP_FILENAME, path.Join(topDir, "top_"+node+".txt"))
			if err != nil {
				log.Fatal("Failed to copy TOP logs from node: ", node, err)
			}
		}(node)
	}
	wg.Wait()
}

/**
* Collects logs from a specific pod
**/
func (m *MetricManager) collectPodLogs(podIP string, podName string, outputDirName string) {
	log.Debugf("Collecting %s logs from %s", podName, podIP)
	outputDir := path.Join(m.outputDir, outputDirName)
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		log.Fatal(err)
	}
	// Retrieve logs
	err = ml_common.CopyRemoteFile(m.multiLoaderConfig.ActivatorNode, fmt.Sprintf("/var/log/pods/knative-serving_%s-*/*/*", podName), outputDir)
	if err != nil {
		log.Fatal("Failed to copy activator logs from node: ", m.multiLoaderConfig.ActivatorNode, err)
	}
	// Check if output dir contains anything
	files, err := os.ReadDir(outputDir)
	if err != nil {
		log.Fatal("Unexpected error. Failed to read directory ", outputDir)
	}
	if len(files) == 0 {
		log.Warnf("No logs were found for pod %s in directory %s", podName, outputDir)
	} else {
		log.Debugf("Successfully retrieved %d logs for pod %s in directory %s", len(files), podName, outputDir)
	}
	m.consolidateLogs(podName, outputDir)
}

func (m *MetricManager) consolidateLogs(podName string, logDir string) {
	files, err := os.ReadDir(logDir)
	if err != nil {
		log.Fatal("Unexpected error. Failed to read directory ", logDir)
	}
	// Remove outdated rotated log files
	thresholdStartTime := m.startTime.Format(TIMESTAMP_FORMAT)
	timestampRegex := regexp.MustCompile(`(\d{8}-\d{6})`)
	log.Debug("Threshold Start timestamp:", thresholdStartTime)
	if err != nil {
		log.Fatal("Error parsing startime: ", m.startTime, err)
	}

	// Filtering out based on file name
	for _, file := range files {
		fileName := file.Name()
		logFilePath := path.Join(logDir, fileName)
		log.Debug("Checking timestamp in filename: ", fileName)

		if file.IsDir() || !strings.HasSuffix(fileName, ".gz") {
			continue
		}

		// Check rotated logs timestamp
		matches := timestampRegex.FindStringSubmatch(fileName)
		if len(matches) > 0 {
			if matches[1] < thresholdStartTime {
				os.Remove(logFilePath)
				continue
			}
		}

		// unzip compressed log file
		_, err := ml_common.DecompressGZFile(logFilePath)
		if err != nil {
			log.Fatal(err)
		}
	}
	// Consolidate current log and rotated log files and remove outdated lines of logs
	logOutputPath := path.Join(logDir, fmt.Sprintf("knative-serving_%s.log", podName))
	outputFile, err := os.Create(logOutputPath)
	if err != nil {
		log.Fatal("Failed to create output file: ", outputFile, err)
	}
	defer outputFile.Close()

	// Filter out logs based on each log line timestamp
	err = filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || path == logOutputPath {
			return err
		}
		log.Debug("Reading and checking timestamps in log file: ", path)

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			// Compare timestamps
			fields := strings.Fields(line)
			if fields[0] > m.startTime.Format(time.RFC3339Nano) {
				if len(fields) > 3 {
					fmt.Fprintln(outputFile, strings.Join(fields[3:], " "))
				}
			}
		}
		return scanner.Err()
	})
	if err != nil {
		log.Printf("Error processing log files: %v", err)
	}
	// Remove all other log files
	files, err = os.ReadDir(logDir)
	if err != nil {
		log.Fatal("Failed to read directory: ", logDir, err)
	}
	for _, file := range files {
		if file.Name() != fmt.Sprintf("knative-serving_%s.log", podName) {
			err := os.RemoveAll(path.Join(logDir, file.Name()))
			if err != nil {
				log.Fatal("Error occured when removing log files", err)
			}
		}
	}
	// Check output
	if stat, _ := outputFile.Stat(); stat.Size() == 0 {
		log.Warnf("No logs found for pod %s after filtering", podName)
	} else {
		log.Debugf("Collected logs for pod %s in %s", podName, logOutputPath)
	}
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
		log.Fatal("Failed to retrieve Prometheus snapshot", err)
		return
	}
	// Check if snapshot status is successful
	if snapshot.Status != "success" {
		log.Fatal("Prometheus snapshot status not successful: ", snapshot)
		return
	}
	// Copy prometheus snapshot to file
	_, err = exec.Command("kubectl", "cp", "-n", "monitoring",
		"prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/",
		"-c", "prometheus", promethOutputDir).CombinedOutput()
	if err != nil {
		log.Fatal("Failed to copy Prometheus snapshot data from monitoring pod using kubectl. ", err)
		return
	}
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
		time.Sleep(100 * time.Millisecond)
	}
	return snapshot, fmt.Errorf("exhausted all attempts to retrieve Prometheus snapshot")
}

/**
 * Helper function to check if the metrics should be collected
 */
func (m *MetricManager) shouldCollect(targetMetrics string) bool {
	// Only collect for Knative
	if !strings.HasPrefix(m.platform, common.PlatformKnative) {
		return false
	}
	for _, metric := range m.metricsToCollect {
		if metric == targetMetrics {
			return true
		}
	}
	return false
}
