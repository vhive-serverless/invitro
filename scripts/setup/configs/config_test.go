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

func validNodeSetupForTest() NodeSetup {
	data, _ := os.ReadFile("node_setup_base_1.json")
	var nodeSetup NodeSetup
	_ = json.Unmarshal(data, &nodeSetup)
	return nodeSetup
}
