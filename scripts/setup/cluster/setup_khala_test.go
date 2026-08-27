package cluster

import (
	"strings"
	"testing"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
)

func TestFlameGraphProvisionCommandPinsCommitAndScripts(t *testing.T) {
	cfg := &configs.SetupConfig{
		FlameGraphRepo:   "https://github.com/brendangregg/FlameGraph.git",
		FlameGraphCommit: "41fee1f99f9276008b7cd112fca19dc3ea84ac32",
	}
	command := flameGraphProvisionCommand(cfg)
	for _, required := range []string{
		cfg.FlameGraphRepo,
		cfg.FlameGraphCommit,
		"checkout --detach",
		"stackcollapse-perf.pl",
		"flamegraph.pl",
	} {
		if !strings.Contains(command, required) {
			t.Fatalf("provision command is missing %q: %s", required, command)
		}
	}
	if strings.Contains(command, "checkout master") {
		t.Fatalf("provision command uses a floating branch: %s", command)
	}
}
