package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
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
	base := options{profile: "4-node", topology: topology, minio: "http://myminio-api.minio.10.200.3.4.sslip.io", result: t.TempDir(), repetitions: 1, latency: 20, memory: 20, warm: 5}
	realPlan, err := makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 20, memory: 20, warm: 5})
	if err != nil || len(realPlan) != 44 {
		t.Fatalf("real plan = %d, %v", len(realPlan), err)
	}
	syntheticPlan, err := makePlan(options{suite: "synthetic", modes: join(syntheticModes), payloads: join(syntheticPayloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 20, memory: 20, warm: 5})
	if err != nil || len(syntheticPlan) != 70 {
		t.Fatalf("synthetic plan = %d, %v", len(syntheticPlan), err)
	}
}

func TestPlanAcceptsConfiguredMinio(t *testing.T) {
	topology := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	_, err = makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: "4-node", topology: topology, minio: "http://minio:9000", result: t.TempDir(), repetitions: 1, latency: 20, memory: 20, warm: 5})
	if err != nil {
		t.Fatal("configured MinIO endpoint rejected:", err)
	}
}

func TestRemoteArgsPropagateSampling(t *testing.T) {
	o := options{suite: "synthetic", modes: join(syntheticModes), payloads: join(syntheticPayloads),
		result: "/mnt/resources/nexus-evaluation/test/e1-synthetic", repetitions: 2,
		latency: 7, memory: 3, warm: 2}
	heads := eval.EvaluationHeads{Khala: "khala-head", Firecracker: "firecracker-head", RDMA: "rdma-head", InVitro: "invitro-head"}
	joined := strings.Join(buildRemoteArgs(o, heads, "/users/nehalem", "http://minio.example:80"), " ")
	for _, required := range []string{
		"/users/nehalem/khala/experiment-script/real-workload/run_nexus_evaluation.sh",
		"--modes " + join(syntheticModes), "--payloads " + join(syntheticPayloads),
		"NEXUS_REPETITIONS_TOTAL=2", "--repetitions 2",
		"--latency-iterations 7", "--memory-iterations 3", "--warm-invocations 2",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("remote args missing %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, "--workloads --payloads") {
		t.Fatalf("remote args contain an empty workloads value: %s", joined)
	}
}

func TestCustomNonSmokePlanUsesRequestedMatrixAndRepetitions(t *testing.T) {
	topology := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := makePlan(options{suite: "synthetic", modes: "invm-go,nexus-go", payloads: "64,4096",
		profile: "4-node", topology: topology, minio: "http://myminio-api.minio.10.200.3.4.sslip.io",
		result: t.TempDir(), repetitions: 2, latency: 7, memory: 3, warm: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 8 || plan[0].Repetition != 0 || plan[len(plan)-1].Repetition != 1 {
		t.Fatalf("custom plan = %#v", plan)
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
