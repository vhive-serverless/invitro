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

func TestCleanupUsesTheEvaluatedMinioEndpoint(t *testing.T) {
	arguments := cleanupCommand([]string{"env"}, cell{Mode: "invm-py"}, eval.CanonicalMinioHost)
	if !slices.Contains(arguments, "--minio-endpoint="+eval.CanonicalMinioHost) {
		t.Fatalf("cleanup arguments = %v", arguments)
	}
}

func TestWorkerEnvironmentKeepsE4CleanupLocal(t *testing.T) {
	arguments := workerEnvironment("/users/tester", eval.CanonicalMinioHost)
	for _, want := range []string{
		"--chdir=/users/tester/khala",
		"KHALA_LOCAL_ONLY=1",
		"KHALA_WORKER_ROOT=/users/tester/khala",
		"NEXUS_MINIO_URL=http://" + eval.CanonicalMinioHost,
	} {
		if !slices.Contains(arguments, want) {
			t.Fatalf("worker environment missing %q: %v", want, arguments)
		}
	}
}

func TestFrozenPlanAlternatesModeOrder(t *testing.T) {
	topology := copyTopology(t)
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: topology,
		MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: "/mnt/resources/nexus-evaluation/test/e4"},
		workloads: join(workloads), modes: join(modes), countsText: "1,2,4,6,8,10,20,30,40", warmup: 1, sampleSeconds: 10}
	plan, _, _, err := makePlan(o)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 22 {
		t.Fatalf("cells = %d, want 22", len(plan))
	}
	if plan[0].Mode != "invm-py" || plan[1].Mode != "nexus-py" || plan[2].Mode != "nexus-py" || plan[3].Mode != "invm-py" {
		t.Fatalf("mode order not alternated: %#v", plan[:4])
	}
}

func TestSmokePlanIsTwoCells(t *testing.T) {
	o := options{common: eval.Config{Profile: eval.Profile4, TopologyConfig: copyTopology(t),
		MinioEndpoint: "http://" + eval.CanonicalMinioHost, ResultRoot: "/mnt/resources/nexus-evaluation/test/e4-smoke"},
		workloads: "helloworld", modes: join(modes), countsText: "1,2", warmup: 1, sampleSeconds: 10, smoke: true}
	plan, _, _, err := makePlan(o)
	if err != nil || len(plan) != 2 {
		t.Fatalf("smoke plan = %d, %v", len(plan), err)
	}
}

func TestB0SetupOrRunFailureCleansAndReachesN4(t *testing.T) {
	for _, stage := range []string{"seed", "density"} {
		t.Run(stage, func(t *testing.T) {
			fixture := newLifecycleFixture(t)
			fixture.failures["invm-py/"+stage] = []error{errors.New("injected " + stage), errors.New("injected " + stage)}
			plan := []cell{
				{Workload: "workload", Mode: "invm-py", Counts: []int{1}},
				{Workload: "workload", Mode: "nexus-py", Counts: []int{1}},
			}
			err := runCells(context.Background(), fixture.options(), plan, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
			if err == nil {
				t.Fatal("expected B0 cell failure")
			}
			if fixture.count("invm-py/cleanup") == 0 {
				t.Fatalf("B0 failure did not clean up: %v", fixture.events)
			}
			if fixture.count("nexus-py/density") != 1 {
				t.Fatalf("N4 did not run after isolated B0 failure: %v", fixture.events)
			}
			if stage == "seed" && fixture.count("invm-py/seed") != 2 {
				t.Fatalf("setup recovery count = %d, want 2: %v", fixture.count("invm-py/seed"), fixture.events)
			}
			if stage == "density" && fixture.count("invm-py/density") != 1 {
				t.Fatalf("density was retried: %v", fixture.events)
			}
			if stage == "density" {
				fixture.requireOrder(t, "invm-py/density", "invm-py/cleanup", "invm-py/copy", "invm-py/verify")
			}
		})
	}
}

func TestPreCleanupFailureStopsBeforeN4(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/cleanup"] = []error{errors.New("injected cleanup")}
	plan := []cell{
		{Workload: "workload", Mode: "invm-py", Counts: []int{1}},
		{Workload: "workload", Mode: "nexus-py", Counts: []int{1}},
	}
	err := runCells(context.Background(), fixture.options(), plan, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil {
		t.Fatal("accepted cleanup failure")
	}
	if fixture.count("nexus-py/absent") != 0 || fixture.count("nexus-py/density") != 0 {
		t.Fatalf("continued after contamination-risk cleanup failure: %v", fixture.events)
	}
	if fixture.count("invm-py/seed") != 0 {
		t.Fatalf("pre-clean failure dispatched setup: %v", fixture.events)
	}
}

func TestSuccessfulCellOrdersCleanupBeforeSetupAndVerification(t *testing.T) {
	fixture := newLifecycleFixture(t)
	item := cell{Workload: "workload", Mode: "invm-py", Counts: []int{1}}
	cleanupOK, err := runCellWith(context.Background(), fixture.options(), item, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err != nil || !cleanupOK {
		t.Fatalf("cell = cleanup=%t err=%v", cleanupOK, err)
	}
	fixture.requireOrder(t, "invm-py/cleanup", "invm-py/seed", "invm-py/deploy", "invm-py/snapshot", "invm-py/density", "invm-py/cleanup", "invm-py/copy", "invm-py/verify")
}

func TestDensityFailureCopiesAndPreservesCompleteArtifacts(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/density"] = []error{errors.New("injected density")}
	item := cell{Workload: "workload", Mode: "invm-py", Counts: []int{1}}
	cleanupOK, err := runCellWith(context.Background(), fixture.options(), item, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil || !cleanupOK {
		t.Fatalf("cell = cleanup=%t err=%v, want terminal run failure after safe cleanup", cleanupOK, err)
	}
	fixture.requireOrder(t, "invm-py/density", "invm-py/cleanup", "invm-py/copy", "invm-py/verify")
	data, readErr := os.ReadFile(filepath.Join(fixture.root, "workload", "invm-py-cell.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var manifest cellManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "failed" || manifest.Phase != "run" || manifest.Artifacts["count-1.manifest"] != "hash" {
		t.Fatalf("terminal run manifest did not preserve complete artifacts: %#v", manifest)
	}
}

func TestRemoteAbsentUsesOnlySetupRecoveryAndRecordsTerminalFailure(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/absent"] = []error{errors.New("injected absent"), errors.New("injected absent")}
	plan := []cell{
		{Workload: "workload", Mode: "invm-py", Counts: []int{1}},
		{Workload: "workload", Mode: "nexus-py", Counts: []int{1}},
	}
	err := runCells(context.Background(), fixture.options(), plan, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil {
		t.Fatal("accepted terminal RemoteAbsent failure")
	}
	if fixture.count("invm-py/absent") != 2 || fixture.count("invm-py/seed") != 0 {
		t.Fatalf("RemoteAbsent recovery violated setup-only retry budget: %v", fixture.events)
	}
	if fixture.count("nexus-py/density") != 1 {
		t.Fatalf("N4 did not continue after terminal RemoteAbsent: %v", fixture.events)
	}
	data, readErr := os.ReadFile(filepath.Join(fixture.root, "workload", "invm-py-cell.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var manifest cellManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "failed" || manifest.Phase != "setup" || manifest.SetupAttempts != 2 {
		t.Fatalf("terminal RemoteAbsent manifest = %#v", manifest)
	}
}

func TestVerificationFailureIsTerminalAndRecorded(t *testing.T) {
	fixture := newLifecycleFixture(t)
	fixture.failures["invm-py/verify"] = []error{errors.New("injected verification")}
	plan := []cell{
		{Workload: "workload", Mode: "invm-py", Counts: []int{1}},
		{Workload: "workload", Mode: "nexus-py", Counts: []int{1}},
	}
	err := runCells(context.Background(), fixture.options(), plan, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops())
	if err == nil {
		t.Fatal("accepted verification failure")
	}
	if fixture.count("invm-py/density") != 1 || fixture.count("invm-py/verify") != 1 {
		t.Fatalf("verification failure retried acquisition: %v", fixture.events)
	}
	if fixture.count("nexus-py/density") != 1 {
		t.Fatalf("N4 did not continue after recorded verification failure: %v", fixture.events)
	}
	data, readErr := os.ReadFile(filepath.Join(fixture.root, "workload", "invm-py-cell.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var manifest cellManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "failed" || manifest.Phase != "verify" || manifest.Acquisition != true {
		t.Fatalf("terminal verification manifest = %#v", manifest)
	}
}

func TestCellArtifactsAreCreateOnly(t *testing.T) {
	fixture := newLifecycleFixture(t)
	item := cell{Workload: "workload", Mode: "invm-py", Counts: []int{1}}
	if cleanupOK, err := runCellWith(context.Background(), fixture.options(), item, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", fixture.ops()); err != nil || !cleanupOK {
		t.Fatalf("first cell = cleanup=%t err=%v", cleanupOK, err)
	}
	manifestPath := filepath.Join(fixture.root, "workload", "invm-py-cell.json")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	second := newLifecycleFixtureAt(t, fixture.root)
	cleanupOK, err := runCellWith(context.Background(), second.options(), item, "tester@127.0.0.1", "/users/tester", "127.0.0.1", eval.CanonicalMinioHost, "campaign", second.ops())
	if err == nil || !cleanupOK {
		t.Fatalf("second cell = cleanup=%t err=%v, want immutable dispatch failure", cleanupOK, err)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("second invocation overwrote the cell manifest")
	}
	if second.count("invm-py/seed") != 0 {
		t.Fatalf("second invocation dispatched despite immutable artifacts: %v", second.events)
	}
}

type lifecycleFixture struct {
	root      string
	events    []string
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

func (f *lifecycleFixture) options() options {
	return options{common: eval.Config{ResultRoot: f.root}}
}

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
		found := -1
		for index := from; index < len(f.events); index++ {
			if f.events[index] == want {
				found = index
				break
			}
		}
		if found < 0 {
			t.Fatalf("event %q missing after index %d: %v", want, from, f.events)
		}
		from = found + 1
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
			mode, stage := modeFromArgs(args), stageFromArgs(args)
			if mode == "unknown" {
				mode = f.mode
			}
			return f.takeFailure(mode + "/" + stage)
		},
		copyTree: func(_ context.Context, _ string, root string, _ io.Writer) error {
			return f.takeFailure(filepath.Base(root) + "/copy")
		},
		verifyCount: func(root string, _ int) (string, string, error) {
			mode := filepath.Base(root)
			if err := f.takeFailure(mode + "/verify"); err != nil {
				return "", "", err
			}
			return filepath.Join(root, "count-1.manifest"), "hash", nil
		},
		createOnly: eval.CreateOnly,
		now: func() time.Time {
			f.timestamp = f.timestamp.Add(time.Second)
			return f.timestamp
		},
	}
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
	case strings.Contains(joined, "--command=clean"):
		return "cleanup"
	default:
		return "unknown"
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

func join(values []string) string { return stringsJoin(values, ",") }

func stringsJoin(values []string, separator string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += separator
		}
		result += value
	}
	return result
}
