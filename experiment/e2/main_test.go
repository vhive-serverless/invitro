package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestSinglePassValidationPrecedesExecution(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0644); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), "collect", options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology, ResultRoot: filepath.Join(dir, "out"), MinioEndpoint: "http://" + eval.CanonicalMinioHost, DryRun: true}, e1Summary: "missing", reference: "missing", replicas: 320, repetitions: 2})
	if err == nil {
		t.Fatal("accepted repetitions != 1")
	}
}

func TestSmokeDryRunUsesBoundedTwoCellAdapter(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0644); err != nil {
		t.Fatal(err)
	}
	err = run(context.Background(), "smoke", options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology,
		ResultRoot: filepath.Join(dir, "out"), MinioEndpoint: "http://" + eval.CanonicalMinioHost, DryRun: true},
		replicas: 2, repetitions: 1, warmupMinutes: 2, measurementMinutes: 1, smoke: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); !os.IsNotExist(err) {
		t.Fatalf("smoke dry-run wrote result root: %v", err)
	}
}
