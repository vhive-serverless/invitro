package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestDryRunPlansWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "result")
	code, err := run(context.Background(), eval.Config{Profile: eval.Profile4, TopologyConfig: topology, MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: result, DryRun: true}, "")
	if err != nil || code != 0 {
		t.Fatalf("dry run = %d, %v", code, err)
	}
	if _, err := os.Stat(result); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote result root: %v", err)
	}
}

func TestSmokeEvidenceRequiresAllFourExperimentSmokes(t *testing.T) {
	root := t.TempDir()
	manifests := map[string]string{
		"e1-2b":   "smoke=true\nclaim_id=e1-smoke-2b\nexit_status=0\n",
		"e1-4mib": "smoke=true\nclaim_id=e1-smoke-4mib\nexit_status=0\n",
		"e2-b0":   "smoke=true\nphase=collection\nworkload=helloworld\nmode=invm-py\nexit_status=0\n",
		"e2-n4":   "smoke=true\nphase=collection\nworkload=helloworld\nmode=nexus-py\nexit_status=0\n",
		"e3-b0":   "smoke=true\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=invm-py\nexit_status=0\n",
		"e3-n4":   "smoke=true\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=nexus-py\nexit_status=0\n",
		"e3-n5":   "smoke=true\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=nexus-rdma-py\nexit_status=0\n",
	}
	for name, content := range manifests {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, mode := range []string{"invm-py", "nexus-py"} {
		content := fmt.Sprintf(`{"status":"complete","cell":{"workload":"helloworld","mode":%q,"counts":[1,2]}}`, mode)
		if err := os.WriteFile(filepath.Join(root, "e4-"+mode+"-cell.json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	rep := report{}
	c := checks{report: &rep}
	c.smokeEvidence(root)
	if c.failed() {
		t.Fatalf("valid smoke rejected: %#v", rep.Checks)
	}
}

func TestParseRemoteHeadIgnoresSSHFirstContactWarning(t *testing.T) {
	output := "Warning: Permanently added 'github.com' (ED25519) to the list of known hosts.\r\n" +
		"d507e07db22243bbb58710345b63991ceb84ba88\trefs/heads/jy/nexus-rdma-eval\n"
	head, err := parseRemoteHead(output, "jy/nexus-rdma-eval")
	if err != nil || head != "d507e07db22243bbb58710345b63991ceb84ba88" {
		t.Fatalf("parseRemoteHead = %q, %v", head, err)
	}
}
