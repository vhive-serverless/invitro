package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFrozenRealAndSyntheticCellCounts(t *testing.T) {
	topology := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	base := options{profile: "4-node", topology: topology, minio: "http://myminio-api.minio.10.200.3.4.sslip.io", result: t.TempDir(), repetitions: 1, latency: 4, memory: 4, warm: 4}
	realPlan, err := makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 4, memory: 4, warm: 4})
	if err != nil || len(realPlan) != 33 {
		t.Fatalf("real plan = %d, %v", len(realPlan), err)
	}
	syntheticPlan, err := makePlan(options{suite: "synthetic", modes: join(allModes), payloads: join(syntheticPayloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 4, memory: 4, warm: 4})
	if err != nil || len(syntheticPlan) != 88 {
		t.Fatalf("synthetic plan = %d, %v", len(syntheticPlan), err)
	}
}

func TestPlanRejectsNonCanonicalMinio(t *testing.T) {
	topology := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: "4-node", topology: topology, minio: "http://minio", result: t.TempDir(), repetitions: 1, latency: 4, memory: 4, warm: 4})
	if err == nil {
		t.Fatal("non-canonical MinIO endpoint accepted")
	}
}

func join(values []string) string {
	result := ""
	for i, value := range values {
		if i > 0 {
			result += ","
		}
		result += value
	}
	return result
}
