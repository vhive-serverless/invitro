package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSelectIDsRequiresCanonicalNine(t *testing.T) {
	if _, err := selectIDs("microbench-grpc"); err == nil {
		t.Fatal("expected incomplete identity error")
	}
	if _, err := selectIDs("microbench-grpc,microbench-grpc,microbench-grpc-s3,microbench-python-loop,microbench-vsock,microbench-vsock-minio,microbench-vsock-s3,microbench-tcp,idle-vm"); err == nil {
		t.Fatal("expected duplicate identity error")
	}
	got, err := selectIDs("microbench-grpc,microbench-grpc-minio,microbench-grpc-s3,microbench-python-loop,microbench-vsock,microbench-vsock-minio,microbench-vsock-s3,microbench-tcp,idle-vm")
	if err != nil || len(got) != 9 {
		t.Fatalf("canonical identities: %v", err)
	}
}

func TestVerifyTerminalResults(t *testing.T) {
	root := t.TempDir()
	o := options{result: root, latency: 1, memory: 1}
	manifest := map[string]any{"status": "complete", "measurement": "both", "identities": identities, "latency_iterations": 1, "memory_iterations": 1, "warm_invocations": 1}
	b, _ := json.Marshal(manifest)
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	latency, err := os.Create(filepath.Join(root, "latency.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	memory, err := os.Create(filepath.Join(root, "memory.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	invokable := map[string]bool{"microbench-python-loop": false, "idle-vm": false}
	for _, id := range identities {
		for _, phase := range []string{"restore", "readiness"} {
			writeTestRow(t, latency, map[string]any{"identity": id, "iteration": 0, "phase": phase, "status": "ok"})
		}
		status := "ok"
		if value, present := invokable[id]; present && !value {
			status = "NA"
		}
		for _, phase := range []string{"cold", "warm"} {
			writeTestRow(t, latency, map[string]any{"identity": id, "iteration": 0, "phase": phase, "status": status})
		}
		writeTestRow(t, memory, map[string]any{"identity": id, "iteration": 0, "phase": "post_restore_steady_state", "status": "ok"})
	}
	latency.Close()
	memory.Close()
	if err := verifyTerminalResults(o); err != nil {
		t.Fatal(err)
	}
}

func writeTestRow(t *testing.T, file *os.File, row any) {
	t.Helper()
	if err := json.NewEncoder(file).Encode(row); err != nil {
		t.Fatal(err)
	}
}
