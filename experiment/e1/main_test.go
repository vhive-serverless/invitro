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
	base := options{profile: "4-node", topology: topology, minio: "http://myminio-api.minio.10.200.3.4.sslip.io", result: t.TempDir(), repetitions: 1, latency: 10, memory: 10, warm: 5}
	realPlan, err := makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 10, memory: 10, warm: 5})
	if err != nil || len(realPlan) != 44 {
		t.Fatalf("real plan = %d, %v", len(realPlan), err)
	}
	syntheticPlan, err := makePlan(options{suite: "synthetic", modes: join(syntheticModes), payloads: join(syntheticPayloads), profile: base.profile, topology: topology, minio: base.minio, result: base.result, repetitions: 1, latency: 10, memory: 10, warm: 5})
	if err != nil || len(syntheticPlan) != 56 {
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
	_, err = makePlan(options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), profile: "4-node", topology: topology, minio: "http://minio", result: t.TempDir(), repetitions: 1, latency: 10, memory: 10, warm: 5})
	if err == nil {
		t.Fatal("non-canonical MinIO endpoint accepted")
	}
}

func TestRemoteArgsPropagateSampling(t *testing.T) {
	o := options{suite: "synthetic", modes: join(syntheticModes), payloads: join(syntheticPayloads),
		result: "/mnt/resources/nexus-evaluation/test/e1-synthetic", repetitions: 1,
		latency: 10, memory: 10, warm: 5}
	heads := eval.EvaluationHeads{Khala: "khala-head", Firecracker: "firecracker-head", RDMA: "rdma-head", InVitro: "invitro-head"}
	joined := strings.Join(buildRemoteArgs(o, heads, "/users/nehalem", "http://minio.example:80"), " ")
	for _, required := range []string{
		"/users/nehalem/khala/experiment-script/real-workload/run_nexus_evaluation.sh",
		"--modes " + join(syntheticModes), "--payloads " + join(syntheticPayloads),
		"VM_SHMEM_BYTES=16777216", "NEXUS_MMAP_BYTES=16777216", "--vm-shmem-bytes 16777216", "--nexus-mmap-bytes 16777216",
		"--latency-iterations 10", "--memory-iterations 10", "--warm-invocations 5",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("remote args missing %q: %s", required, joined)
		}
	}
	if strings.Contains(joined, "--workloads --payloads") {
		t.Fatalf("remote args contain an empty workloads value: %s", joined)
	}
}

func TestRemoteArgsUseFourMiBForRealSuite(t *testing.T) {
	o := options{suite: "real", modes: join(realModes), workloads: join(realWorkloads), result: "/tmp/e1-real-plan", repetitions: 1, latency: 10, memory: 10, warm: 5}
	heads := eval.EvaluationHeads{Khala: "k", Firecracker: "f", RDMA: "r", InVitro: "i"}
	joined := strings.Join(buildRemoteArgs(o, heads, "/users/nehalem", "http://minio.example:80"), " ")
	for _, required := range []string{"VM_SHMEM_BYTES=4194304", "NEXUS_MMAP_BYTES=4194304", "--vm-shmem-bytes 4194304", "--nexus-mmap-bytes 4194304"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("real remote args missing %q: %s", required, joined)
		}
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
