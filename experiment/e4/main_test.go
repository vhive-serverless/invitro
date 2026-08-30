package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vhive-serverless/loader/experiment/eval"
)

func TestCleanupUsesEndpointAndRequestedSnapshotPolicy(t *testing.T) {
	for _, tc := range []struct {
		remove bool
		want   string
	}{{true, "--remove-snapshots=true"}, {false, "--remove-snapshots=false"}} {
		args := cleanupCommand([]string{"env"}, cell{Mode: "invm-py"}, eval.CanonicalMinioHost, tc.remove)
		for _, want := range []string{"--minio-endpoint=" + eval.CanonicalMinioHost, tc.want} {
			if !slices.Contains(args, want) {
				t.Fatalf("cleanup arguments missing %q: %v", want, args)
			}
		}
	}
}

func TestWorkerEnvironmentKeepsE4CleanupLocal(t *testing.T) {
	args := workerEnvironment("/users/tester", eval.CanonicalMinioHost)
	for _, want := range []string{
		"--chdir=/users/tester/khala", "KHALA_LOCAL_ONLY=1",
		"KHALA_WORKER_ROOT=/users/tester/khala", "NEXUS_MINIO_URL=http://" + eval.CanonicalMinioHost,
	} {
		if !slices.Contains(args, want) {
			t.Fatalf("worker environment missing %q: %v", want, args)
		}
	}
}

func TestFrozenPlanHasTwoAggregateModeCells(t *testing.T) {
	plan, _, _, err := makePlan(claimOptions(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("cells = %d, want 2", len(plan))
	}
	for index, mode := range modes {
		item := plan[index]
		if item.Mode != mode || !slices.Equal(item.Workloads, workloads) || !slices.Equal(item.InstancesPerWorkload, counts) {
			t.Fatalf("cell %d = %#v", index, item)
		}
	}
}

func TestSmokePlanIsTwoHelloWorldCells(t *testing.T) {
	o := claimOptions(t)
	o.workloads, o.countsText, o.smoke = "helloworld", "1,2", true
	plan, _, _, err := makePlan(o)
	if err != nil || len(plan) != 2 {
		t.Fatalf("smoke plan = %d, %v", len(plan), err)
	}
	for _, item := range plan {
		if !slices.Equal(item.Workloads, []string{"helloworld"}) || !slices.Equal(item.InstancesPerWorkload, []int{1, 2}) {
			t.Fatalf("smoke cell = %#v", item)
		}
	}
}

func TestInitialCleanupPurgesSnapshotsAndIsCreateOnly(t *testing.T) {
	fixture := newLifecycleFixture(t)
	o := fixture.options()
	if err := runInitialCleanup(context.Background(), o, "tester@127.0.0.1", "/users/tester", eval.CanonicalMinioHost, "campaign", fixture.ops()); err != nil {
		t.Fatal(err)
	}
	if fixture.count("nexus-py/cleanup-purge") != 1 {
		t.Fatalf("initial cleanup events = %v", fixture.events)
	}
	var manifest initialCleanupManifest
	readJSON(t, filepath.Join(fixture.root, "initial-cleanup.json"), &manifest)
	if manifest.Status != "complete" || manifest.LogSHA256 == "" || !manifest.RemoveSnapshots || manifest.SnapshotPolicy != snapshotCleanupPolicy {
		t.Fatalf("initial cleanup manifest = %#v", manifest)
	}
	before, _ := os.ReadFile(filepath.Join(fixture.root, "initial-cleanup.json"))
	if err := runInitialCleanup(context.Background(), o, "tester@127.0.0.1", "/users/tester", eval.CanonicalMinioHost, "campaign", fixture.ops()); err == nil {
		t.Fatal("second initial cleanup overwrote create-only evidence")
	}
	after, _ := os.ReadFile(filepath.Join(fixture.root, "initial-cleanup.json"))
	if string(before) != string(after) {
		t.Fatal("second initial cleanup changed evidence")
	}
}

func TestSuccessfulAggregateCellSetsUpOnceAndPreservesSnapshots(t *testing.T) {
	fixture := newLifecycleFixture(t)
	item := testCell("invm-py")
	cleanupOK, err := runCellWith(context.Background(), fixture.options(), item, workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", false, fixture.ops())
	if err != nil || !cleanupOK {
		t.Fatalf("cell = cleanup=%t err=%v", cleanupOK, err)
	}
	fixture.requireOrder(t, "invm-py/seed", "invm-py/deploy", "invm-py/snapshot", "invm-py/snapshot", "invm-py/density", "invm-py/cleanup-preserve", "invm-py/copy", "invm-py/verify")
	if got := fixture.snapshotWorkloads(); !slices.Equal(got, item.Workloads) {
		t.Fatalf("snapshots = %v, want %v", got, item.Workloads)
	}
	var manifest cellManifest
	readJSON(t, filepath.Join(fixture.root, "invm-py-cell.json"), &manifest)
	if manifest.ManifestVersion != 3 || manifest.Status != "complete" || manifest.Phase != "verify" || !manifest.Acquisition || !manifest.CleanupSucceeded || !manifest.VerificationDone || manifest.SnapshotPolicy != snapshotCleanupPolicy {
		t.Fatalf("complete manifest = %#v", manifest)
	}
}

func TestSuccessfulPlanPurgesSnapshotsOnlyAfterFinalCell(t *testing.T) {
	fixture := newLifecycleFixture(t)
	if err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops()); err != nil {
		t.Fatal(err)
	}
	if fixture.count("invm-py/cleanup-preserve") != 1 || fixture.count("invm-py/cleanup-purge") != 0 {
		t.Fatalf("non-final cleanup policy = %v", fixture.events)
	}
	if fixture.count("nexus-py/cleanup-purge") != 1 || fixture.count("nexus-py/cleanup-preserve") != 0 {
		t.Fatalf("final cleanup policy = %v", fixture.events)
	}
}

func TestFinalSnapshotPurgeFailurePreventsSuccessfulCampaign(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["nexus-py/cleanup-purge"] = []error{errors.New("snapshot purge")}
	err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil || fixture.count("nexus-py/density") != 1 || fixture.count("nexus-py/cleanup-purge") != 1 {
		t.Fatalf("final purge failure did not fail campaign: err=%v events=%v", err, fixture.events)
	}
	var manifest cellManifest
	readJSON(t, filepath.Join(fixture.root, "nexus-py-cell.json"), &manifest)
	if manifest.Phase != "cleanup" || manifest.CleanupSucceeded {
		t.Fatalf("final purge failure manifest = %#v", manifest)
	}
}

func TestSetupFailureGetsOnePurgeRecoveryAndReachesN4(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/seed"] = []error{errors.New("first"), errors.New("second")}
	err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil || fixture.count("invm-py/seed") != 2 || fixture.count("invm-py/cleanup-purge") != 2 || fixture.count("nexus-py/density") != 1 {
		t.Fatalf("setup recovery/continuation: err=%v events=%v", err, fixture.events)
	}
}

func TestDensityFailureIsNeverRetriedAndReachesN4AfterCleanup(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/density"] = []error{errors.New("density")}
	err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil || fixture.count("invm-py/density") != 1 || fixture.count("nexus-py/density") != 1 {
		t.Fatalf("acquisition retry/continuation: err=%v events=%v", err, fixture.events)
	}
	fixture.requireOrder(t, "invm-py/density", "invm-py/cleanup-preserve", "invm-py/copy", "invm-py/verify", "nexus-py/density")
	var manifest cellManifest
	readJSON(t, filepath.Join(fixture.root, "invm-py-cell.json"), &manifest)
	if manifest.Phase != "run" || !manifest.CleanupSucceeded || !manifest.VerificationDone || manifest.Artifacts["count-1.manifest"] != "hash" {
		t.Fatalf("terminal run manifest = %#v", manifest)
	}
}

func TestCleanupFailureIsContaminationHardStop(t *testing.T) {
	for _, stage := range []string{"cleanup-purge", "cleanup-preserve"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newLifecycleFixture(t)
			if stage == "cleanup-purge" {
				fixture.failures["invm-py/seed"] = []error{errors.New("setup")}
			}
			fixture.failures["invm-py/"+stage] = []error{errors.New("cleanup")}
			err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops())
			if err == nil || fixture.count("nexus-py/density") != 0 {
				t.Fatalf("cleanup failure did not hard-stop: err=%v events=%v", err, fixture.events)
			}
		})
	}
}

func TestRecoverablePreCellAndVerificationFailuresRemainIndependent(t *testing.T) {
	for _, stage := range []string{"absent", "verify"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newLifecycleFixture(t)
			fixture.failures["invm-py/"+stage] = []error{errors.New(stage), errors.New(stage)}
			err := runCells(context.Background(), fixture.options(), testPlan(), workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", fixture.ops())
			if err == nil || fixture.count("nexus-py/density") != 1 {
				t.Fatalf("isolated failure stopped N4: err=%v events=%v", err, fixture.events)
			}
			if stage == "absent" && (fixture.count("invm-py/absent") != 2 || fixture.count("invm-py/seed") != 0) {
				t.Fatalf("pre-cell recovery exceeded setup budget: %v", fixture.events)
			}
			if stage == "verify" && (fixture.count("invm-py/density") != 1 || fixture.count("invm-py/verify") != 1) {
				t.Fatalf("verification retried acquisition: %v", fixture.events)
			}
		})
	}
}

func TestCellArtifactsAreCreateOnly(t *testing.T) {
	fixture := newLifecycleFixture(t)
	item := testCell("invm-py")
	if ok, err := runCellWith(context.Background(), fixture.options(), item, workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", false, fixture.ops()); err != nil || !ok {
		t.Fatalf("first cell = cleanup=%t err=%v", ok, err)
	}
	path := filepath.Join(fixture.root, "invm-py-cell.json")
	before, _ := os.ReadFile(path)
	second := newLifecycleFixtureAt(t, fixture.root)
	ok, err := runCellWith(context.Background(), second.options(), item, workerURL, workerHome, workerIP, eval.CanonicalMinioHost, "campaign", false, second.ops())
	if err == nil || !ok || second.count("invm-py/seed") != 0 {
		t.Fatalf("second cell = cleanup=%t err=%v events=%v", ok, err, second.events)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("second invocation changed immutable cell evidence")
	}
}

const (
	workerURL  = "tester@127.0.0.1"
	workerHome = "/users/tester"
	workerIP   = "127.0.0.1"
)

type lifecycleFixture struct {
	root      string
	events    []string
	commands  [][]string
	failures  map[string][]error
	timestamp time.Time
	mode      string
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	return newLifecycleFixtureAt(t, t.TempDir())
}
func newLifecycleFixtureAt(t *testing.T, root string) *lifecycleFixture {
	t.Helper()
	return &lifecycleFixture{root: root, failures: map[string][]error{}, timestamp: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)}
}
func (f *lifecycleFixture) options() options { return options{common: eval.Config{ResultRoot: f.root}} }
func (f *lifecycleFixture) count(event string) int {
	count := 0
	for _, got := range f.events {
		if got == event {
			count++
		}
	}
	return count
}
func (f *lifecycleFixture) requireOrder(t *testing.T, sequence ...string) {
	t.Helper()
	from := 0
	for _, want := range sequence {
		found := slices.Index(f.events[from:], want)
		if found < 0 {
			t.Fatalf("event %q missing after index %d: %v", want, from, f.events)
		}
		from += found + 1
	}
}
func (f *lifecycleFixture) takeFailure(event string) error {
	f.events = append(f.events, event)
	values := f.failures[event]
	if len(values) == 0 {
		return nil
	}
	f.failures[event] = values[1:]
	return values[0]
}
func (f *lifecycleFixture) ops() cellOps {
	return cellOps{
		remoteAbsent: func(_ context.Context, _ string, root string) error {
			f.mode = filepath.Base(root)
			return f.takeFailure(f.mode + "/absent")
		},
		runRemote: func(_ context.Context, _ string, _ io.Writer, args ...string) error {
			f.commands = append(f.commands, slices.Clone(args))
			mode, stage := modeFromArgs(args), stageFromArgs(args)
			if mode == "unknown" {
				mode = f.mode
			}
			return f.takeFailure(mode + "/" + stage)
		},
		copyTree: func(_ context.Context, _ string, root string, _ io.Writer) error {
			return f.takeFailure(filepath.Base(root) + "/copy")
		},
		verifyCount: func(root string, count int) (string, string, error) {
			mode := filepath.Base(root)
			if err := f.takeFailure(mode + "/verify"); err != nil {
				return "", "", err
			}
			return filepath.Join(root, "count-"+renderCounts([]int{count})+".manifest"), "hash", nil
		},
		createOnly: eval.CreateOnly,
		now: func() time.Time {
			f.timestamp = f.timestamp.Add(time.Second)
			return f.timestamp
		},
	}
}
func (f *lifecycleFixture) snapshotWorkloads() []string {
	var out []string
	for _, args := range f.commands {
		if stageFromArgs(args) != "snapshot" {
			continue
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "--workload=") {
				out = append(out, strings.TrimPrefix(arg, "--workload="))
			}
		}
	}
	return out
}

func modeFromArgs(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--mode=") {
			return strings.TrimPrefix(arg, "--mode=")
		}
	}
	return "unknown"
}
func stageFromArgs(args []string) string {
	joined := strings.Join(args, " ")
	switch {
	case strings.Contains(joined, "deploy-minio-obj.sh"):
		return "seed"
	case strings.Contains(joined, "--command=create-snapshots"):
		return "snapshot"
	case strings.Contains(joined, "--command=deploy"):
		return "deploy"
	case strings.Contains(joined, "./bin/e4-density"):
		return "density"
	case strings.Contains(joined, "--command=clean") && strings.Contains(joined, "--remove-snapshots=true"):
		return "cleanup-purge"
	case strings.Contains(joined, "--command=clean"):
		return "cleanup-preserve"
	default:
		return "unknown"
	}
}

func claimOptions(t *testing.T) options {
	t.Helper()
	return options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: copyTopology(t), MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: "/mnt/resources/nexus-evaluation/test/e4"}, workloads: strings.Join(workloads, ","), modes: strings.Join(modes, ","), countsText: renderCounts(counts), warmup: 1, sampleSeconds: 10}
}
func testCell(mode string) cell {
	return cell{Workloads: []string{"one", "two"}, Mode: mode, InstancesPerWorkload: []int{1}}
}
func testPlan() []cell { return []cell{testCell("invm-py"), testCell("nexus-py")} }
func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatal(err)
	}
}
func copyTopology(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "topology.json")
	data, err := os.ReadFile("../../scripts/setup/configs/node_setup_base_1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
