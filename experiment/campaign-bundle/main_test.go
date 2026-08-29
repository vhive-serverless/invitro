package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func putFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func restoreWritable(path string) {
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err == nil {
			if info.IsDir() {
				_ = os.Chmod(p, 0755)
			} else {
				_ = os.Chmod(p, 0644)
			}
		}
		return nil
	})
}

func putCleanFinalLeakCheck(t *testing.T, root, uid string, generation int64) {
	t.Helper()
	check := eval.FinalLeakCheck{
		Version: 1, Status: "PASS", CapturedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Worker:     eval.WorkerLeakEvidence{Firecracker: []string{}, KnIntegration: []string{}, NexusBackend: []string{}},
		Storage:    eval.StorageLeakEvidence{RDMAServer: []string{}, RDMASessions: []string{}},
		Kubernetes: eval.KubernetesLeakEvidence{KSVCCount: 0},
		Snapshots:  eval.SnapshotLeakEvidence{Entries: []string{}, Bytes: 0},
		Activator:  eval.ActivatorIdentity{UID: uid, Generation: generation},
	}
	if err := eval.WriteFinalLeakCheck(filepath.Join(root, finalLeakCheckName), check, check.Activator); err != nil {
		t.Fatal(err)
	}
}

func TestTreeDigestDeterministic(t *testing.T) {
	a := []record{{Path: "z", SHA256: strings.Repeat("a", 64), Bytes: 1}, {Path: "a", SHA256: strings.Repeat("b", 64), Bytes: 2}}
	b := []record{{Path: "a", SHA256: strings.Repeat("b", 64), Bytes: 2}, {Path: "z", SHA256: strings.Repeat("a", 64), Bytes: 1}}
	// Canonicalization sorts the complete "sha256  path" records, independent
	// of the order in which a filesystem walk discovers files.
	if treeDigest(a) != treeDigest(b) {
		t.Fatal("canonical digest depends on record order")
	}
}

func TestScanRejectsSymlinkAndSpecial(t *testing.T) {
	d := t.TempDir()
	putFile(t, filepath.Join(d, "ok"), "ok")
	if err := os.Symlink(filepath.Join(d, "ok"), filepath.Join(d, "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := scan(d, nil); err == nil {
		t.Fatal("scan accepted symlink")
	}
	_ = os.Remove(filepath.Join(d, "link"))
	if err := os.Mkdir(filepath.Join(d, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	// A directory is valid; this also ensures traversal does not mistake it
	// for a special file.
	if _, err := scan(d, nil); err != nil {
		t.Fatal(err)
	}
}

func TestScanRejectsHardlinkOutsideTree(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(base, "original")
	putFile(t, original, "same inode")
	if err := os.Link(original, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := scan(root, nil); err == nil {
		t.Fatal("scan accepted an externally hardlinked file")
	}
}

func TestCopyTreePopulatesThenRestoresReadOnlyModes(t *testing.T) {
	base := t.TempDir()
	defer restoreWritable(base)
	src := filepath.Join(base, "src")
	dst := filepath.Join(base, "dst")
	file := filepath.Join(src, "readonly", "nested", "payload")
	putFile(t, file, "payload")
	if err := os.Chmod(file, 0444); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(src, "readonly", "nested"), filepath.Join(src, "readonly"), src} {
		if err := os.Chmod(path, 0555); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := scan(src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, dst, expected); err != nil {
		t.Fatal(err)
	}
	actual, err := scan(dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sameInventory(expected, actual) {
		t.Fatal("read-only materialization changed the byte inventory")
	}
	for _, path := range []string{dst, filepath.Join(dst, "readonly"), filepath.Join(dst, "readonly", "nested")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0555 {
			t.Fatalf("%s mode = %o, want 0555", path, info.Mode().Perm())
		}
	}
	info, err := os.Stat(filepath.Join(dst, "readonly", "nested", "payload"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0444 {
		t.Fatalf("payload mode = %o, want 0444", info.Mode().Perm())
	}
}

func TestPlanMaterializeSealVerifyAndTamper(t *testing.T) {
	base := t.TempDir()
	defer restoreWritable(base)
	q := filepath.Join(base, "qualification")
	old := filepath.Join(base, "old")
	newRoot := filepath.Join(base, "new")
	for _, p := range []string{q, old, newRoot} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(old, "a.txt"), "alpha")
	putFile(t, filepath.Join(old, "nested", "b.txt"), "beta")
	oldCampaign := `{"status":"ACQUISITION_START","provenance":[{"branch":"main","head":"abc"}],"marker":"<"}`
	oldCampaignPath := filepath.Join(old, "campaign.json")
	putFile(t, oldCampaignPath, oldCampaign)
	if err := planReuse(q, oldCampaignPath, []string{"artifact=" + old}); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(q, ledgerName)
	if _, err := os.Stat(ledgerPath); err != nil {
		t.Fatal(err)
	}
	qInv, err := scan(q, nil)
	if err != nil {
		t.Fatal(err)
	}
	newCampaignPath := filepath.Join(newRoot, "campaign.json")
	manifest := struct {
		Status              string `json:"status"`
		AcquisitionStart    string `json:"acquisition_start"`
		QualificationRoot   string `json:"qualification_root"`
		QualificationSHA256 string `json:"qualification_sha256"`
		ActivatorUID        string `json:"activator_uid"`
		ActivatorGeneration int64  `json:"activator_generation"`
	}{"ACQUISITION_START", "2026-08-29T00:00:00Z", q, qInv.TreeSHA, "activator-uid", 7}
	mb, _ := json.Marshal(manifest)
	putFile(t, newCampaignPath, string(mb))
	if err := materializeReuse(ledgerPath, newCampaignPath, newRoot); err != nil {
		t.Fatal(err)
	}
	srcInv, _ := scan(old, nil)
	dstInv, err := scan(filepath.Join(newRoot, "artifact"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !sameInventory(srcInv, dstInv) {
		t.Fatal("materialized tree differs from source")
	}
	putCleanFinalLeakCheck(t, newRoot, "activator-uid", 7)
	if err := sealBundle(newRoot); err != nil {
		t.Fatal(err)
	}
	if err := verifyBundle(newRoot); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(newRoot, "artifact", "a.txt"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "artifact", "a.txt"), []byte("tampered"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := verifyBundle(newRoot); err == nil {
		t.Fatal("verify accepted tampered file")
	}
}

func TestMaterializeRejectsQualificationBinding(t *testing.T) {
	base := t.TempDir()
	q := filepath.Join(base, "q")
	old := filepath.Join(base, "old")
	n := filepath.Join(base, "n")
	for _, p := range []string{q, old, n} {
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
	}
	putFile(t, filepath.Join(old, "x"), "x")
	oldCampaign := filepath.Join(old, "campaign.json")
	putFile(t, oldCampaign, `{}`)
	if err := planReuse(q, oldCampaign, []string{"x=" + old}); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(n, "campaign.json"), `{"qualification_root":"wrong","qualification_sha256":"bad"}`)
	if err := materializeReuse(filepath.Join(q, ledgerName), filepath.Join(n, "campaign.json"), n); err == nil {
		t.Fatal("materialize accepted wrong qualification binding")
	}
}

func TestVerifyRejectsUnindexedExtra(t *testing.T) {
	d := t.TempDir()
	defer restoreWritable(d)
	putFile(t, filepath.Join(d, "campaign.json"), `{"status":"ACQUISITION_START","acquisition_start":"2026-08-29T00:00:00Z","activator_uid":"activator-uid","activator_generation":7}`)
	putFile(t, filepath.Join(d, "x"), `x`)
	putCleanFinalLeakCheck(t, d, "activator-uid", 7)
	if err := sealBundle(d); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(d, 0755); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(d, "extra"), `extra`)
	if err := verifyBundle(d); err == nil {
		t.Fatal("verify accepted unindexed extra")
	}
}

func TestSealRequiresFinalLeakCheck(t *testing.T) {
	d := t.TempDir()
	defer restoreWritable(d)
	putFile(t, filepath.Join(d, "campaign.json"), `{"status":"ACQUISITION_START","acquisition_start":"2026-08-29T00:00:00Z","activator_uid":"activator-uid","activator_generation":7}`)
	putFile(t, filepath.Join(d, "x"), `x`)
	if err := sealBundle(d); err == nil || !strings.Contains(err.Error(), "final leak-check") {
		t.Fatalf("seal without cleanup evidence: %v", err)
	}
}

func TestSealRejectsCleanupLeak(t *testing.T) {
	d := t.TempDir()
	defer restoreWritable(d)
	putFile(t, filepath.Join(d, "campaign.json"), `{"status":"ACQUISITION_START","acquisition_start":"2026-08-29T00:00:00Z","activator_uid":"activator-uid","activator_generation":7}`)
	putFile(t, filepath.Join(d, "x"), `x`)
	check := eval.FinalLeakCheck{
		Version: 1, Status: "PASS", CapturedUTC: time.Now().UTC().Format(time.RFC3339Nano),
		Worker:     eval.WorkerLeakEvidence{Firecracker: []string{"pid=9 firecracker"}, KnIntegration: []string{}, NexusBackend: []string{}},
		Storage:    eval.StorageLeakEvidence{RDMAServer: []string{}, RDMASessions: []string{}},
		Kubernetes: eval.KubernetesLeakEvidence{}, Snapshots: eval.SnapshotLeakEvidence{Entries: []string{}, Bytes: 0},
		Activator: eval.ActivatorIdentity{UID: "activator-uid", Generation: 7},
	}
	data, err := json.Marshal(check)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(d, finalLeakCheckName), string(data))
	if err := sealBundle(d); err == nil || !strings.Contains(err.Error(), "leak") {
		t.Fatalf("seal with cleanup leak: %v", err)
	}
}

func TestLeakCheckFixtureInvokesOnlyReadOnlyProbes(t *testing.T) {
	root := t.TempDir()
	setup := eval.Setup{NodeLabel: map[string][]string{
		"loader-nodetype=worker": {"10.0.1.3"}, "minio-type=tenant": {"10.0.1.4"},
	}, NodeURL: []string{"nehalem@master", "nehalem@loader", "nehalem@worker", "nehalem@storage"}}
	campaign := frozenCampaign{Status: "ACQUISITION_START", AcquisitionStart: "2026-08-29T00:00:00Z", ActivatorUID: "activator-uid", ActivatorGeneration: 7, Topology: setup}
	data, err := json.Marshal(campaign)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(root, "campaign.json"), string(data))

	oldSSH, oldActivator, oldKSVC := readonlySSHProbe, activatorIdentityProbe, ksvcProbe
	defer func() { readonlySSHProbe, activatorIdentityProbe, ksvcProbe = oldSSH, oldActivator, oldKSVC }()
	var commands [][]string
	readonlySSHProbe = func(_ context.Context, target string, args ...string) (string, error) {
		commands = append(commands, append([]string{target}, args...))
		return "", nil
	}
	activatorIdentityProbe = func(context.Context) (eval.ActivatorIdentity, error) {
		return eval.ActivatorIdentity{UID: "activator-uid", Generation: 7}, nil
	}
	ksvcProbe = func(context.Context) (string, error) { return "", nil }
	if err := captureFinalLeakCheck(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, finalLeakCheckName)); err != nil {
		t.Fatal(err)
	}
	for _, command := range commands {
		joined := strings.Join(command, " ")
		for _, forbidden := range []string{"delete", "patch", "restart", "rollout", "apply", "create", "replace"} {
			if strings.Contains(strings.ToLower(joined), forbidden) {
				t.Fatalf("mutating token %q in probe command %q", forbidden, joined)
			}
		}
	}
	for _, command := range [][]string{workerProcessProbeCommand(), storageProcessProbeCommand(), storageSessionProbeCommand(), snapshotListProbeCommand("/safe/path"), snapshotSizeProbeCommand("/safe/path/file"), ksvcProbeCommand()} {
		joined := strings.Join(command, " ")
		if strings.Contains(joined, "delete") || strings.Contains(joined, "restart") || strings.Contains(joined, "patch") {
			t.Fatalf("mutating command constructed: %q", joined)
		}
	}
}

func TestSnapshotProbeParsers(t *testing.T) {
	if lines := nonEmptyLines("\n/users/nehalem/khala/runtime/snapshots/mapper.mem\n"); len(lines) != 1 {
		t.Fatalf("snapshot entries = %v", lines)
	}
	if bytes, err := parseSnapshotSize("12\n"); err != nil || bytes != 12 {
		t.Fatalf("snapshot size = %d, %v", bytes, err)
	}
	if _, err := parseSnapshotSize("not-a-size"); err == nil {
		t.Fatal("malformed snapshot size accepted")
	}
}
