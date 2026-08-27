package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestFourNodeRequiresPilotLabel(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0644); err != nil {
		t.Fatal(err)
	}
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology, ResultRoot: filepath.Join(dir, "out"), MinioEndpoint: "http://" + eval.CanonicalMinioHost, DryRun: true}, modes: "invm-py,nexus-py,nexus-rdma-py", reference: "ref", startScale: 1, step: 1, endScale: 27, shiftStep: 10, divisor: 100, warmupMinutes: 2, repetitions: 1}
	if err := run(context.Background(), o); err == nil {
		t.Fatal("accepted unlabeled four-node E3")
	}
}

func TestSmokeRequiresEndScaleOneAndZeroCooldown(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0644); err != nil {
		t.Fatal(err)
	}
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology, ResultRoot: filepath.Join(dir, "out"), MinioEndpoint: "http://" + eval.CanonicalMinioHost, DryRun: true}, modes: "invm-py,nexus-py,nexus-rdma-py", reference: "ref", startScale: 1, step: 1, endScale: 27, shiftStep: 10, divisor: 100, warmupMinutes: 2, repetitions: 1, campaignLabel: "4-node-pilot", pilotRun: true, smoke: true}
	if err := run(context.Background(), o); err == nil {
		t.Fatal("accepted unbounded E3 smoke")
	}
}
