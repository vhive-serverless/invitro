package eval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cleanLeakCheck() FinalLeakCheck {
	return FinalLeakCheck{
		Version: 1, Status: "PASS", CapturedUTC: "2026-08-29T00:00:00Z",
		Worker:     WorkerLeakEvidence{Firecracker: []string{}, KnIntegration: []string{}, NexusBackend: []string{}},
		Storage:    StorageLeakEvidence{RDMAServer: []string{}, RDMASessions: []string{}},
		Kubernetes: KubernetesLeakEvidence{KSVCCount: 0}, Snapshots: SnapshotLeakEvidence{Entries: []string{}, Bytes: 0},
		Activator: ActivatorIdentity{UID: "uid-1", Generation: 4},
	}
}

func TestParseActivatorIdentityRejectsMissingOrMalformed(t *testing.T) {
	for _, input := range []string{"", "uid-1", "uid-1 nope", "uid-1 0", "uid 1 extra"} {
		if _, err := ParseActivatorIdentity(input); err == nil {
			t.Fatalf("accepted malformed activator identity %q", input)
		}
	}
	got, err := ParseActivatorIdentity("uid-1\t4\n")
	if err != nil || got != (ActivatorIdentity{UID: "uid-1", Generation: 4}) {
		t.Fatalf("identity = %#v, %v", got, err)
	}
}

func TestFinalLeakCheckRejectsActivatorMismatchAndLeaks(t *testing.T) {
	baseline := ActivatorIdentity{UID: "uid-1", Generation: 4}
	check := cleanLeakCheck()
	if err := check.ValidateFinalLeakCheck(baseline); err != nil {
		t.Fatal(err)
	}
	check.Activator.Generation++
	if err := check.ValidateFinalLeakCheck(baseline); err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("activator mismatch accepted: %v", err)
	}
	check = cleanLeakCheck()
	check.Worker.Firecracker = []string{"firecracker --api-sock ..."}
	if err := check.ValidateFinalLeakCheck(baseline); err == nil {
		t.Fatal("worker process leak accepted")
	}
	check = cleanLeakCheck()
	check.Kubernetes.KSVCCount = 1
	if err := check.ValidateFinalLeakCheck(baseline); err == nil {
		t.Fatal("ksvc leak accepted")
	}
}

func TestFinalLeakCheckRejectsMissingEvidence(t *testing.T) {
	check := cleanLeakCheck()
	check.Snapshots.Entries = nil
	if err := check.ValidateFinalLeakCheck(check.Activator); err == nil {
		t.Fatal("missing snapshot evidence accepted")
	}
	check = cleanLeakCheck()
	check.Storage.RDMASessions = nil
	if err := check.ValidateFinalLeakCheck(check.Activator); err == nil {
		t.Fatal("missing RDMA session evidence accepted")
	}
}

func TestWriteFinalLeakCheckIsCreateOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campaign-final-leak-check.json")
	check := cleanLeakCheck()
	if err := WriteFinalLeakCheck(path, check, check.Activator); err != nil {
		t.Fatal(err)
	}
	if err := WriteFinalLeakCheck(path, check, check.Activator); err == nil {
		t.Fatal("final leak check was overwritten")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
