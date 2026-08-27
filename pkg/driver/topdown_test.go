package driver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/pkg/config"
)

func TestPerfOutputPrefixIsAbsolute(t *testing.T) {
	got, err := perfOutputPrefix("results/experiment")
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) || !strings.HasSuffix(got, filepath.Join("results", "experiment")) {
		t.Fatalf("unexpected prefix %q", got)
	}
}

func TestStopPerfCollectionReportsMissingArtifactsAndUsesAbsoluteDestination(t *testing.T) {
	originalServerExec, originalSleep := serverExec, perfCollectionSleep
	t.Cleanup(func() { serverExec, perfCollectionSleep = originalServerExec, originalSleep })
	perfCollectionSleep = func(_ time.Duration) {}
	var commands []string
	serverExec = func(_, command string) (string, error) {
		commands = append(commands, command)
		return "", nil
	}
	prefix := filepath.Join(t.TempDir(), "experiment")
	ctx := &PerfCollectionContext{
		cfg:           config.Configuration{LoaderConfiguration: &config.LoaderConfiguration{OutputPathPrefix: prefix}},
		workerNodeIps: []string{"10.0.1.3"}, loaderNodeIp: "10.0.1.2",
		cancelChannels: []chan struct{}{make(chan struct{})},
	}
	err := StopPerfCollection(ctx)
	if err == nil || !strings.Contains(err.Error(), "missing or empty perf artifact") {
		t.Fatalf("missing artifacts were not reported: %v", err)
	}
	joined := strings.Join(commands, "\n")
	if strings.Contains(joined, "~/loader/") || !strings.Contains(joined, fmt.Sprintf("10.0.1.2:%s_perf_0.csv", prefix)) {
		t.Fatalf("unexpected rsync destinations:\n%s", joined)
	}
}

func TestPerfOutputPrefixRejectsEmpty(t *testing.T) {
	if _, err := perfOutputPrefix(""); err == nil {
		t.Fatal("expected empty prefix error")
	}
}

func TestPerfArtifactPathsAreDistinct(t *testing.T) {
	paths := perfArtifactPaths("/tmp/experiment", 2)
	if len(paths) != 4 {
		t.Fatalf("got %d artifacts", len(paths))
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			t.Fatalf("duplicate artifact %q", path)
		}
		seen[path] = true
	}
	if _, err := os.Stat(paths[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected test artifact: %v", err)
	}
}

func TestCollectionErrorAggregatesFailures(t *testing.T) {
	p := &PerfCollectionContext{}
	p.addError(errors.New("stat failed"))
	p.addError(errors.New("record failed"))
	err := p.collectionError()
	if err == nil || !strings.Contains(err.Error(), "stat failed") || !strings.Contains(err.Error(), "record failed") {
		t.Fatalf("unexpected aggregate error: %v", err)
	}
}
