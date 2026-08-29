package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestDryRunPlansWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	topology := filepath.Join(dir, "topology.json")
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "setup", "configs", "node_setup_base_1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topology, data, 0600); err != nil {
		t.Fatal(err)
	}
	result := filepath.Join(dir, "result")
	code, err := run(context.Background(), eval.Config{Profile: eval.Profile4, TopologyConfig: topology, MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: result, DryRun: true}, "")
	if err != nil || code != 0 {
		t.Fatalf("dry run = %d, %v", code, err)
	}
	if _, err := os.Stat(result); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote result root: %v", err)
	}
}

func TestFreezeCapturesActivatorBaselineFailClosed(t *testing.T) {
	old := captureActivatorIdentity
	defer func() { captureActivatorIdentity = old }()
	rep := report{}
	captureActivatorIdentity = func(context.Context) (eval.ActivatorIdentity, error) {
		return eval.ActivatorIdentity{UID: "activator-uid", Generation: 9}, nil
	}
	if err := captureCampaignActivatorBaseline(context.Background(), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.ActivatorUID != "activator-uid" || rep.ActivatorGeneration != 9 {
		t.Fatalf("baseline = %q/%d", rep.ActivatorUID, rep.ActivatorGeneration)
	}
	captureActivatorIdentity = func(context.Context) (eval.ActivatorIdentity, error) {
		return eval.ActivatorIdentity{}, nil
	}
	rep = report{}
	if err := captureCampaignActivatorBaseline(context.Background(), &rep); err == nil {
		t.Fatal("malformed activator baseline accepted")
	}
	if rep.ActivatorUID != "" || rep.ActivatorGeneration != 0 {
		t.Fatalf("malformed baseline was persisted: %q/%d", rep.ActivatorUID, rep.ActivatorGeneration)
	}
}

func TestSmokeEvidenceRequiresAllFourExperimentSmokes(t *testing.T) {
	root := t.TempDir()
	cleanupLog := filepath.Join(root, "initial-cleanup.log")
	if err := os.WriteFile(cleanupLog, []byte("COMMAND cleanup\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cleanupHash, err := eval.SHA256File(cleanupLog)
	if err != nil {
		t.Fatal(err)
	}
	cleanupRecord := fmt.Sprintf(`{"manifest_version":1,"status":"complete","log_sha256":%q,"remove_snapshots":true,"snapshot_cleanup_policy":%q}`, cleanupHash, e4SnapshotCleanupPolicy)
	if err := os.WriteFile(filepath.Join(root, "initial-cleanup.json"), []byte(cleanupRecord), 0644); err != nil {
		t.Fatal(err)
	}
	manifests := map[string]string{
		"e1-2b":   "smoke=true\nmanifest_version=9\nclaim_id=e1-smoke-2b\ncell_status_sequence=started,complete\nfixture_setup_max_attempts=2\nfixture_setup_attempts=1\ncell_setup_max_attempts=2\nacquisition_retry=false\nindependent_continuation=true\ncontamination_stop=true\nexit_status=0\n",
		"e1-4mib": "smoke=true\nmanifest_version=9\nclaim_id=e1-smoke-4mib\ncell_status_sequence=started,complete\nfixture_setup_max_attempts=2\nfixture_setup_attempts=1\ncell_setup_max_attempts=2\nacquisition_retry=false\nindependent_continuation=true\ncontamination_stop=true\nexit_status=0\n",
		"e2-b0":   "smoke=true\nmanifest_version=2\nphase=collection\nworkload=helloworld\nmode=invm-py\nevidence_status=0\nsetup_attempts=1\ndeploy_attempts=1\ndeploy_invocations=1\nloader_started=true\ncleanup_exit_status=0\nlifecycle_phase=final\nexit_status=0\n",
		"e2-n4":   "smoke=true\nmanifest_version=2\nphase=collection\nworkload=helloworld\nmode=nexus-py\nevidence_status=0\nsetup_attempts=1\ndeploy_attempts=1\ndeploy_invocations=1\nloader_started=true\ncleanup_exit_status=0\nlifecycle_phase=final\nexit_status=0\n",
		"e3-b0":   "smoke=true\nmanifest_version=2\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=invm-py\nsetup_attempts=1\ndeploy_attempts=1\ndeploy_invocations=1\nloader_started=true\ncleanup_exit_status=0\nlifecycle_phase=final\nexit_status=0\n",
		"e3-n4":   "smoke=true\nmanifest_version=2\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=nexus-py\nsetup_attempts=1\ndeploy_attempts=1\ndeploy_invocations=1\nloader_started=true\ncleanup_exit_status=0\nlifecycle_phase=final\nexit_status=0\n",
		"e3-n5":   "smoke=true\nmanifest_version=2\nexperiment=e3\nend_scale=1\nclaim_bearing=false\nmode=nexus-rdma-py\nsetup_attempts=1\ndeploy_attempts=1\ndeploy_invocations=1\nloader_started=true\ncleanup_exit_status=0\nlifecycle_phase=final\nexit_status=0\n",
	}
	for name, content := range manifests {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.txt"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		if name[:2] == "e2" || name[:2] == "e3" {
			writeChecksumFixture(t, dir)
		}
	}
	for _, mode := range []string{"invm-py", "nexus-py"} {
		content := fmt.Sprintf(`{"manifest_version":3,"status":"complete","phase":"verify","setup_attempts":1,"acquisition_started":true,"cleanup_succeeded":true,"verification_completed":true,"snapshot_cleanup_policy":"initial-purge;normal-preserve;setup-recovery-purge;campaign-final-purge","cell":{"workloads":["helloworld"],"mode":%q,"instances_per_workload":[1,2]}}`, mode)
		if err := os.WriteFile(filepath.Join(root, "e4-"+mode+"-cell.json"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	rep := report{}
	c := checks{report: &rep}
	c.smokeEvidence(root)
	if c.failed() {
		t.Fatalf("valid smoke rejected: %#v", rep.Checks)
	}
	if rep.QualificationRoot != root || len(rep.QualificationSHA256) != 64 {
		t.Fatalf("qualification binding = %q, %q", rep.QualificationRoot, rep.QualificationSHA256)
	}
}

func TestArchivedOutputChecksumsRejectHeaderOnlyAndCorruption(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "archived-output-checksums.csv"), []byte("path,sha256\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchivedOutputChecksums(dir); err == nil {
		t.Fatal("accepted a header-only checksum archive")
	}
	writeChecksumFixture(t, dir)
	if err := validateArchivedOutputChecksums(dir); err != nil {
		t.Fatalf("valid checksum archive rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload.txt"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := validateArchivedOutputChecksums(dir); err == nil {
		t.Fatal("accepted a corrupted archived payload")
	}
}

func writeChecksumFixture(t *testing.T, dir string) {
	t.Helper()
	payload := filepath.Join(dir, "payload.txt")
	if err := os.WriteFile(payload, []byte("qualification\n"), 0644); err != nil {
		t.Fatal(err)
	}
	digest, err := eval.SHA256File(payload)
	if err != nil {
		t.Fatal(err)
	}
	content := "path,sha256\npayload.txt," + digest + "\n"
	if err := os.WriteFile(filepath.Join(dir, "archived-output-checksums.csv"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestParseRemoteHeadIgnoresSSHFirstContactWarning(t *testing.T) {
	output := "Warning: Permanently added 'github.com' (ED25519) to the list of known hosts.\r\n" +
		"d507e07db22243bbb58710345b63991ceb84ba88\trefs/heads/jy/nexus-rdma-eval\n"
	head, err := parseRemoteHead(output, "jy/nexus-rdma-eval")
	if err != nil || head != "d507e07db22243bbb58710345b63991ceb84ba88" {
		t.Fatalf("parseRemoteHead = %q, %v", head, err)
	}
}

func TestMatchingSHA256RequiresLoaderWorkerParity(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	local, remote, err := matchingSHA256(digest+"  ../khala/bin/nexus-backend", digest+"  /users/nehalem/khala/bin/nexus-backend")
	if err != nil || local != digest || remote != digest {
		t.Fatalf("matchingSHA256 match = %q, %q, %v", local, remote, err)
	}
	const other = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if _, _, err := matchingSHA256(digest+"  local", other+"  remote"); err == nil {
		t.Fatal("matchingSHA256 accepted mismatched loader/worker artifacts")
	}
}

func TestParseRuntimeSnapshotsOutputAllowsGitkeepOnly(t *testing.T) {
	path := "/users/nehalem/khala/runtime/snapshots"
	output := "Warning: Permanently added 'worker' (ED25519) to the list of known hosts.\n" + path + "/.gitkeep\n"
	if err := parseRuntimeSnapshotsOutput(output, path); err != nil {
		t.Fatalf(".gitkeep-only snapshots rejected: %v", err)
	}
	if err := parseRuntimeSnapshotsOutput("", path); err != nil {
		t.Fatalf("empty snapshots directory rejected: %v", err)
	}
}

func TestParseRuntimeSnapshotsOutputRejectsStaleEntry(t *testing.T) {
	path := "/users/nehalem/khala/runtime/snapshots"
	err := parseRuntimeSnapshotsOutput(path+"/stale.snapshot\n", path)
	if err == nil || !strings.Contains(err.Error(), "stale.snapshot") {
		t.Fatalf("stale snapshots accepted or poorly reported: %v", err)
	}
}

func TestRuntimeSnapshotsCommandQuotesPathAndUsesMissingAsPass(t *testing.T) {
	path := "/users/worker name/khala/runtime/snapshots;unexpected"
	args := runtimeSnapshotsCommand(path)
	if len(args) != 5 || args[0] != "sh" || args[1] != "-c" || args[3] != "preflight-runtime-snapshots" {
		t.Fatalf("runtime snapshots command = %#v", args)
	}
	if !strings.Contains(args[2], `if [ ! -d "$1" ]; then exit 0; fi`) || !strings.Contains(args[2], `find "$1"`) {
		t.Fatalf("runtime snapshots command does not guard/find through positional path: %q", args[2])
	}
	if args[4] != "'/users/worker name/khala/runtime/snapshots;unexpected'" {
		t.Fatalf("runtime snapshots path is not shell-quoted: %q", args[4])
	}
}
