package metric

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/vhive-serverless/loader/tools/multi_loader/types"
)

func TestLogConsolidation(t *testing.T) {
	t.Run("Log Consolidation Test", func(t *testing.T) {
		metricManager := NewMetricManager("Knative", types.MultiLoaderConfiguration{})

		startTime := time.Now()
		podName := "test"
		metricManager.startTime = startTime
		logDir := "./logs"
		logData := []string{"line1", "line2", "line3"}
		logFileName := []string{"0.log", "11.log", "12.log"}
		// create log dir
		os.Mkdir(logDir, 0755)
		defer os.RemoveAll(logDir)

		// Create log files to be consolidated, 2 zipped, 1 not
		// Create old rotated log
		var b bytes.Buffer
		w := gzip.NewWriter(&b)
		for i, line := range logData {
			w.Write([]byte(fmt.Sprintf("%s 1 2 %s_%s\n", startTime.Add(-time.Minute*time.Duration(i+1)).Format(time.RFC3339Nano), logFileName[0], line)))
		}
		w.Close()
		os.WriteFile(strings.Join([]string{path.Join(logDir, logFileName[0]), (startTime.Add(-time.Minute * 1)).Format(TIMESTAMP_FORMAT), "gz"}, "."), b.Bytes(), 0666)

		// Create valid rotated log with mixed of old and new timestamp
		b.Reset()
		w = gzip.NewWriter(&b)
		for i, line := range logData {
			w.Write([]byte(fmt.Sprintf("%s 1 2 %s_%s\n", startTime.Add(time.Minute*time.Duration(i)-time.Second*30).Format(time.RFC3339Nano), logFileName[1], line)))
		}
		w.Close()
		os.WriteFile(strings.Join([]string{path.Join(logDir, logFileName[1]), (startTime.Add(time.Minute*time.Duration(2) - time.Second*30)).Format(TIMESTAMP_FORMAT), "gz"}, "."), b.Bytes(), 0666)

		// Create current log
		f, err := os.OpenFile(path.Join(logDir, logFileName[2]), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			assert.Fail(t, err.Error())
		}
		for i, line := range logData {
			f.Write([]byte(fmt.Sprintf("%s 1 2 %s_%s\n", startTime.Add(time.Minute*time.Duration(i+3)).Format(time.RFC3339Nano), logFileName[2], line)))
		}
		f.Close()

		metricManager.consolidateLogs(podName, logDir)
		files, _ := os.ReadDir(logDir)
		// logs from 0.log should not be included
		// should contain only line2 and line3 from 11.log
		// should contain all logs from 12.log
		expectedLines := []string{"11.log_line2", "11.log_line3", "12.log_line1", "12.log_line2", "12.log_line3"}

		// should only have one file after consolidation
		assert.True(t, len(files) == 1)
		assert.True(t, files[0].Name() == "knative-serving_test.log")
		// check file content
		content, err := os.ReadFile(path.Join(logDir, files[0].Name()))
		if err != nil {
			assert.Fail(t, err.Error())
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range expectedLines {
			assert.Contains(t, lines, line, fmt.Sprintf("Expected line %d: %s", i, line))
		}
	})
}
