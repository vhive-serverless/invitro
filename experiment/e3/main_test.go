package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestCustomModeSubsetsAreValid(t *testing.T) {
	for _, modes := range []string{"invm-py", "nexus-py,nexus-rdma-py"} {
		if err := validateModes(modes); err != nil {
			t.Fatalf("custom modes %q: %v", modes, err)
		}
	}
	for _, modes := range []string{"", "nexus-py,nexus-py", "unknown"} {
		if err := validateModes(modes); err == nil {
			t.Fatalf("accepted invalid modes %q", modes)
		}
	}
}

func TestModeOrderValidation(t *testing.T) {
	for _, value := range []string{"rotate", "fixed"} {
		if err := validateModeOrder(value); err != nil {
			t.Fatalf("valid mode order %q: %v", value, err)
		}
	}
	for _, value := range []string{"", "counterbalance", "FIXED"} {
		if err := validateModeOrder(value); err == nil {
			t.Fatalf("accepted invalid mode order %q", value)
		}
	}
}

func TestCommandArgsForwardModeOrder(t *testing.T) {
	o := options{common: eval.Config{Profile: eval.Profile10, ResultRoot: "/results", DryRun: true}, modes: "nexus-rdma-py,nexus-py,invm-py", modeOrder: "fixed", reference: "/inputs/reference.csv", startScale: 1, step: 1, endScale: 60, shiftStep: 10, divisor: 50, warmupMinutes: 2, repetitions: 2, cooldownSeconds: 120, allowExtendedEnd: true}
	args := commandArgs(o, "minio:80")
	want := []string{"--mode-order", "fixed"}
	found := false
	for index := range args[:len(args)-1] {
		if reflect.DeepEqual(args[index:index+2], want) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("mode order was not forwarded: %v", args)
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
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology, ResultRoot: filepath.Join(dir, "out"), MinioEndpoint: "http://" + eval.CanonicalMinioHost, DryRun: true}, modes: "invm-py,nexus-py,nexus-rdma-py", modeOrder: "rotate", reference: "ref", startScale: 1, step: 1, endScale: 27, shiftStep: 10, divisor: 100, warmupMinutes: 2, repetitions: 1, campaignLabel: "4-node-pilot", pilotRun: true, smoke: true}
	if err := run(context.Background(), o); err == nil {
		t.Fatal("accepted unbounded E3 smoke")
	}
}
