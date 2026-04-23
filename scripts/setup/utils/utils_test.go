package utils

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
)

func TestCreateTempNodeSetupPreservesNodeSetupJSONContract(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	nodes := []string{
		"master@example.com",
		"loader@example.com",
		"worker1@example.com",
		"worker2@example.com",
	}

	configName, err := CreateTempNodeSetup(configDir, nodes)
	if err != nil {
		t.Fatalf("CreateTempNodeSetup returned error: %v", err)
	}

	if configName != "node_setup_temp.json" {
		t.Fatalf("CreateTempNodeSetup returned unexpected file name: %s", configName)
	}

	intNodeSetup, extNodeSetup, err := configs.GetNodeSetup(configDir, configName)
	if err != nil {
		t.Fatalf("GetNodeSetup returned error for generated temp config: %v", err)
	}

	if _, err := os.Stat(filepath.Join(configDir, configName)); err != nil {
		t.Fatalf("generated config file is missing: %v", err)
	}

	expectedInternalWorkers := []string{"10.0.1.2", "10.0.1.3", "10.0.1.4"}
	if !reflect.DeepEqual(intNodeSetup.NodeSetup.MasterNode, []string{"10.0.1.1"}) {
		t.Fatalf("unexpected internal master node: %#v", intNodeSetup.NodeSetup.MasterNode)
	}
	if !reflect.DeepEqual(intNodeSetup.NodeSetup.LoaderNode, []string{"10.0.1.2"}) {
		t.Fatalf("unexpected internal loader node: %#v", intNodeSetup.NodeSetup.LoaderNode)
	}
	if !reflect.DeepEqual(intNodeSetup.NodeSetup.WorkerNode, expectedInternalWorkers) {
		t.Fatalf("unexpected internal worker nodes: %#v", intNodeSetup.NodeSetup.WorkerNode)
	}

	if !reflect.DeepEqual(extNodeSetup.NodeSetup.MasterNode, []string{"master@example.com"}) {
		t.Fatalf("unexpected external master node: %#v", extNodeSetup.NodeSetup.MasterNode)
	}
	if !reflect.DeepEqual(extNodeSetup.NodeSetup.LoaderNode, []string{"loader@example.com"}) {
		t.Fatalf("unexpected external loader node: %#v", extNodeSetup.NodeSetup.LoaderNode)
	}
	if !reflect.DeepEqual(extNodeSetup.NodeSetup.WorkerNode, []string{"loader@example.com", "worker1@example.com", "worker2@example.com"}) {
		t.Fatalf("unexpected external worker nodes: %#v", extNodeSetup.NodeSetup.WorkerNode)
	}

	expectedLabels := map[string][]string{
		"loader-nodetype=master":     {"master@example.com"},
		"loader-nodetype=monitoring": {"loader@example.com"},
		"loader-nodetype=worker":     {"worker1@example.com", "worker2@example.com"},
	}
	if !reflect.DeepEqual(extNodeSetup.NodeLabel, expectedLabels) {
		t.Fatalf("unexpected external node labels: %#v", extNodeSetup.NodeLabel)
	}
}

func TestCreateTempNodeSetupRequiresMasterAndLoader(t *testing.T) {
	t.Parallel()

	_, err := CreateTempNodeSetup(t.TempDir(), []string{"master@example.com"})
	if err == nil {
		t.Fatal("CreateTempNodeSetup returned nil error for incomplete node list")
	}
}
