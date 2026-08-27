package configs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBaseFourNodeSetupMapsRolesAndURLs(t *testing.T) {
	path := filepath.Join("node_setup_base_1.json")
	internal, external, err := GetNodeSetup(".", path)
	if err != nil {
		t.Fatal(err)
	}
	if got := external.NodeSetup.LoaderNode[0]; got != "nehalem@er089.utah.cloudlab.us" {
		t.Fatalf("loader URL = %q", got)
	}
	if got := external.NodeSetup.WorkerNode[1]; got != "nehalem@er061.utah.cloudlab.us" {
		t.Fatalf("worker URL = %q", got)
	}
	if got := internal.NodeSetup.MinioTenantNode[0]; got != "10.0.1.4" {
		t.Fatalf("tenant IP = %q", got)
	}
}

func TestValidateNodeSetupRejectsRoleLabelOverlap(t *testing.T) {
	nodeSetup := validNodeSetupForTest()
	nodeSetup.NodeLabel["minio-type=tenant"] = []string{"10.0.1.3"}
	if err := validateNodeSetup(&nodeSetup); err == nil {
		t.Fatal("worker/tenant label overlap accepted")
	}
}

func TestValidateNodeSetupRejectsUnmappedIP(t *testing.T) {
	nodeSetup := validNodeSetupForTest()
	nodeSetup.NodeSetup.WorkerNode = []string{"10.0.1.9"}
	if err := validateNodeSetup(&nodeSetup); err == nil {
		t.Fatal("unmapped worker IP accepted")
	}
}

func TestSetupConfigPinsEvaluationBranches(t *testing.T) {
	config, err := GetSetupJSON(".")
	if err != nil {
		t.Fatal(err)
	}
	if config.LoaderBranch != "jy/khala-asplos-27" || config.KhalaBranch != "jy/asplos-26" || config.FirecrackerBranch != "firecracker-v1.14-nexus-shmem-vsock" || config.RDMABranch != "jy/nexus-rdma-eval" {
		t.Fatalf("unexpected evaluation branches: loader=%q khala=%q firecracker=%q rdma=%q", config.LoaderBranch, config.KhalaBranch, config.FirecrackerBranch, config.RDMABranch)
	}
	if config.FlameGraphRepo != "https://github.com/brendangregg/FlameGraph.git" || config.FlameGraphCommit != "41fee1f99f9276008b7cd112fca19dc3ea84ac32" {
		t.Fatalf("unexpected FlameGraph pin: repo=%q commit=%q", config.FlameGraphRepo, config.FlameGraphCommit)
	}
}

func TestValidateSetupConfigRequiresRDMARevision(t *testing.T) {
	config := validSetupConfigForTest()
	config.DeployRDMA = true
	if err := validateSetupConfig(&config); err == nil {
		t.Fatal("missing RDMA revision accepted")
	}
}

func TestValidateSetupConfigRejectsNonCommitFlameGraphRevision(t *testing.T) {
	config := validSetupConfigForTest()
	config.FlameGraphCommit = "master"
	if err := validateSetupConfig(&config); err == nil {
		t.Fatal("floating FlameGraph revision accepted")
	}
}

func validSetupConfigForTest() SetupConfig {
	return SetupConfig{
		HiveRepo: "v", HiveBranch: "v", LoaderRepo: "l", LoaderBranch: "l",
		KhalaRepo: "k", KhalaBranch: "k", FirecrackerRepo: "f", FirecrackerBranch: "f",
		FlameGraphRepo:   "https://example.invalid/FlameGraph.git",
		FlameGraphCommit: "0123456789abcdef0123456789abcdef01234567",
	}
}

func validNodeSetupForTest() NodeSetup {
	data, _ := os.ReadFile("node_setup_base_1.json")
	var nodeSetup NodeSetup
	_ = json.Unmarshal(data, &nodeSetup)
	return nodeSetup
}
