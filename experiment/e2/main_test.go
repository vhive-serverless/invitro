package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

func TestRealCollectCapacityGatePrecedesRunner(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0644); err != nil {
		t.Fatal(err)
	}
	campaign := filepath.Join(dir, "campaign.json")
	campaignData, err := json.Marshal(eval.Campaign{
		Status:              "ACQUISITION_START",
		AcquisitionStart:    "2026-01-01T00:00:00Z",
		ActivatorUID:        "9e6e2b67-8b3f-4841-9029-5f1957a561b0",
		ActivatorGeneration: 7,
		Provenance: []eval.Provenance{
			{Branch: eval.KhalaBranch, Head: "khala"},
			{Branch: eval.InVitroBranch, Head: "invitro"},
			{Branch: eval.RDMABranch, Head: "rdma"},
			{Branch: eval.FirecrackerBranch, Head: "firecracker"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(campaign, campaignData, 0644); err != nil {
		t.Fatal(err)
	}
	resultRoot := filepath.Join(dir, "out")
	err = runWithWorkerPodDiscovery(context.Background(), "collect", options{
		common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology, ResultRoot: resultRoot,
			MinioEndpoint: "http://" + eval.CanonicalMinioHost, CampaignManifest: campaign},
		e1Summary: "e1.csv", reference: "reference.csv", replicas: 320, repetitions: 1,
	}, func(context.Context) (int, error) { return 359, nil })
	if err == nil || !strings.Contains(err.Error(), "allocatable pod capacity 359 < required 360") {
		t.Fatalf("capacity gate error = %v", err)
	}
	if _, statErr := os.Stat(resultRoot); !os.IsNotExist(statErr) {
		t.Fatalf("runner created result root: %v", statErr)
	}
}

func TestWorkerPodCapacityReserve(t *testing.T) {
	if err := requireWorkerPodCapacity(context.Background(), 320, func(context.Context) (int, error) { return 359, nil }); err == nil {
		t.Fatal("accepted capacity without the 40-pod reserve")
	}
	if err := requireWorkerPodCapacity(context.Background(), 320, func(context.Context) (int, error) { return 360, nil }); err != nil {
		t.Fatalf("rejected capacity with required reserve: %v", err)
	}
}

func TestSmokeDefaultsRemainBounded(t *testing.T) {
	o := defaultOptions("smoke")
	if !o.smoke || o.replicas != 2 || o.warmupMinutes != 2 || o.measurementMinutes != 1 {
		t.Fatalf("unexpected smoke defaults: %+v", o)
	}
}
