package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveExperimentModes(t *testing.T) {
	tests := []struct {
		name, tcp, backend string
		sdk, rpc, rdma     bool
		workloads          []string
	}{
		{ModeInVMPy, "guest", "none", false, false, false, []string{"pyaesserve", "mapper", "reducer"}},
		{ModeNexusGo, "guest", "stream", true, true, false, []string{"gopyaesserve", "gomapper", "goreducer"}},
		{ModeNexusRDMA, "guest", "rdma", true, true, true, []string{"gopyaesserve", "gomapper", "goreducer"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, err := resolveExperimentMode(test.name, 4, 256*1024)
			if err != nil {
				t.Fatal(err)
			}
			if mode.TCPTransport != test.tcp || mode.BackendTransport != test.backend ||
				mode.SetNexusSDK != test.sdk || mode.SetNexusRPC != test.rpc || mode.WithRDMA != test.rdma {
				t.Fatalf("unexpected mode: %+v", mode)
			}
			if strings.Join(mode.Workloads, ",") != strings.Join(test.workloads, ",") {
				t.Fatalf("workloads = %v, want %v", mode.Workloads, test.workloads)
			}
		})
	}
}

func TestResolveExperimentModeRejectsAmbiguity(t *testing.T) {
	for _, name := range []string{"", "nexus", "async", "hosttcp-go"} {
		if _, err := resolveExperimentMode(name, 4, 256*1024); err == nil {
			t.Fatalf("resolveExperimentMode(%q) succeeded", name)
		}
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 0, 256*1024); err == nil {
		t.Fatal("zero slots accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 1, 256*1024); err == nil {
		t.Fatal("single stream slot accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 64, 256*1024); err == nil {
		t.Fatal("stream layout larger than the backing accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, int(^uint(0)>>1), int(^uint(0)>>1)); err == nil {
		t.Fatal("overflow-sized stream layout accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusRDMA, 1, 256*1024); err != nil {
		t.Fatalf("RDMA incorrectly subjected to stream slot reservation: %v", err)
	}
}

func TestResolveCleanupModeValidatesOnlyTeardownIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		withRDMA bool
	}{
		{ModeInVMPy, false},
		{ModeNexusGo, false},
		{ModeNexusRDMA, true},
	} {
		mode, err := resolveCleanupMode(test.name)
		if err != nil {
			t.Fatalf("resolveCleanupMode(%q): %v", test.name, err)
		}
		if mode.WithRDMA != test.withRDMA {
			t.Fatalf("resolveCleanupMode(%q).WithRDMA = %t, want %t", test.name, mode.WithRDMA, test.withRDMA)
		}
		if mode.StreamSlots != 0 || mode.StreamCapacity != 0 || mode.BackendTransport != "" || len(mode.Workloads) != 0 {
			t.Fatalf("cleanup resolved irrelevant deployment state for %q: %+v", test.name, mode)
		}
	}
	if _, err := resolveCleanupMode("historical-unknown-mode"); err == nil {
		t.Fatal("unknown cleanup mode accepted")
	}
}

func TestRunCommandCleanupBypassesHistoricalStreamValidation(t *testing.T) {
	originalCommand, originalMode := *Command, *Mode
	originalCorePoolPolicy := *CorePoolPolicy
	originalImplementation, originalSlots, originalCapacity := *Implementation, *StreamSlots, *StreamCapacity
	originalDryRun, originalRemoveSnapshots := *DryRun, *RemoveSnapshots
	originalGetWorkers, originalClean := getWorkerNodesFn, cleanKhalaFn
	t.Cleanup(func() {
		*Command, *Mode = originalCommand, originalMode
		*CorePoolPolicy = originalCorePoolPolicy
		*Implementation, *StreamSlots, *StreamCapacity = originalImplementation, originalSlots, originalCapacity
		*DryRun, *RemoveSnapshots = originalDryRun, originalRemoveSnapshots
		getWorkerNodesFn, cleanKhalaFn = originalGetWorkers, originalClean
	})

	workerSetup := WorkerNodeSetup{WorkerNodes: []string{"worker"}, StorageNodes: []string{"storage"}}
	getWorkerNodesFn = func() (WorkerNodeSetup, error) { return workerSetup, nil }
	*Command, *Implementation, *DryRun, *RemoveSnapshots = "clean", "historical-implementation", false, true
	*CorePoolPolicy = "historical-policy"

	tests := []struct {
		name            string
		slots, capacity int
		wantRDMA        bool
	}{
		{ModeNexusGo, 1, 256 * 1024, false},
		{ModeNexusGo, int(^uint(0) >> 1), int(^uint(0) >> 1), false},
		{ModeNexusRDMA, 0, -1, true},
	}
	for _, test := range tests {
		t.Run(test.name+fmt.Sprintf("-%d", test.slots), func(t *testing.T) {
			*Mode, *StreamSlots, *StreamCapacity = test.name, test.slots, test.capacity
			called := false
			cleanKhalaFn = func(got WorkerNodeSetup, removeSnapshots, withRDMA bool) error {
				called = true
				if strings.Join(got.WorkerNodes, ",") != "worker" || !removeSnapshots || withRDMA != test.wantRDMA {
					t.Fatalf("cleanup args = %+v, remove=%t, rdma=%t", got, removeSnapshots, withRDMA)
				}
				return nil
			}
			if err := runCommand(); err != nil {
				t.Fatalf("cleanup rejected irrelevant stream layout: %v", err)
			}
			if !called {
				t.Fatal("cleanup was not performed")
			}
		})
	}

	*Mode, *StreamSlots, *StreamCapacity = "unknown", 1, int(^uint(0)>>1)
	cleanKhalaFn = func(WorkerNodeSetup, bool, bool) error {
		t.Fatal("cleanup executed for unknown mode")
		return nil
	}
	if err := runCommand(); err == nil {
		t.Fatal("unknown cleanup mode accepted")
	}
}

func TestDryRunLocalFlagsAreValidatedWithoutExternalState(t *testing.T) {
	for _, test := range []struct {
		command, policy, implementation string
	}{
		{"unknown", "", "go"},
		{"deploy", "bad-policy", "go"},
		{"deploy", "", "python"},
		{"deploy", "", "cpp"},
	} {
		if err := validateLocalFlags(test.command, test.policy, test.implementation); err == nil {
			t.Fatalf("validateLocalFlags(%q, %q, %q) succeeded", test.command, test.policy, test.implementation)
		}
	}
	if err := validateLocalFlags("set-corepool", "", "cpp"); err != nil {
		t.Fatalf("set-corepool incorrectly treated as an evaluated mode: %v", err)
	}
	if err := validateLocalFlags("clean", "historical-policy", "historical-implementation"); err != nil {
		t.Fatalf("cleanup rejected irrelevant deployment flags: %v", err)
	}
}

func TestInvalidInvocationStopsBeforeWorkerConfiguration(t *testing.T) {
	originalCommand, originalMode := *Command, *Mode
	originalImplementation := *Implementation
	originalGetWorkers := getWorkerNodesFn
	t.Cleanup(func() {
		*Command, *Mode, *Implementation = originalCommand, originalMode, originalImplementation
		getWorkerNodesFn = originalGetWorkers
	})

	var workerReads atomic.Int32
	getWorkerNodesFn = func() (WorkerNodeSetup, error) {
		workerReads.Add(1)
		return WorkerNodeSetup{}, errors.New("must not be called")
	}
	*Command, *Mode, *Implementation = "deploy", "invalid-backend-mode", "go"
	if err := runCommand(); err == nil {
		t.Fatal("invalid mode accepted")
	}
	if got := workerReads.Load(); got != 0 {
		t.Fatalf("worker configuration read %d times for invalid mode", got)
	}

	*Mode, *Implementation = ModeNexusGo, "cpp"
	if err := runCommand(); err == nil {
		t.Fatal("conflicting implementation accepted")
	}
	if got := workerReads.Load(); got != 0 {
		t.Fatalf("worker configuration read %d times for conflicting implementation", got)
	}
}

func TestSnapshotNames(t *testing.T) {
	tests := map[string][]string{
		ModeInVMPy:    {"pyaesserve-0", "mapper-0", "reducer-0"},
		ModeNexusGo:   {"gopyaesserve-s3-rpc-stream-0", "gomapper-s3-rpc-stream-0", "goreducer-s3-rpc-stream-0"},
		ModeNexusRDMA: {"gopyaesserve-s3-rpc-0", "gomapper-s3-rpc-0", "goreducer-s3-rpc-0"},
	}
	for name, want := range tests {
		mode, _ := resolveExperimentMode(name, 4, 256*1024)
		got := buildSnapshotNames(mode)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s snapshots = %v, want %v", name, got, want)
		}
	}
}

func TestDeploymentCommandCarriesExplicitPlacement(t *testing.T) {
	mode, _ := resolveExperimentMode(ModeNexusGo, 3, 131072)
	command := buildDeploymentCommand("", "go", mode, false)
	for _, fragment := range []string{
		"--tcp-transport=guest", "--backend-transport=stream",
		"--set-nexus-sdk=true", "--set-nexus-rpc=true", "--with-rdma=false",
		"--stream-slots=3", "--stream-capacity=131072",
	} {
		if !strings.Contains(command, fragment) {
			t.Errorf("deployment command %q missing %q", command, fragment)
		}
	}
}

func TestDryRunPlanIsModeAware(t *testing.T) {
	mode, _ := resolveExperimentMode(ModeNexusRDMA, 4, 256*1024)
	plan := buildDryRunPlan(mode, "", "go", false)
	if !plan.CleanupRDMA || !strings.Contains(plan.DeploymentCommand, "--backend-transport=rdma") {
		t.Fatalf("unexpected dry-run plan: %+v", plan)
	}
}

func TestCleanupDryRunPlanContainsOnlyTeardownIdentity(t *testing.T) {
	mode, err := resolveCleanupMode(ModeNexusRDMA)
	if err != nil {
		t.Fatal(err)
	}
	plan := CleanupDryRunPlan{Mode: mode.Name, CleanupRDMA: mode.WithRDMA}
	if plan.Mode != ModeNexusRDMA || !plan.CleanupRDMA {
		t.Fatalf("unexpected cleanup plan: %+v", plan)
	}
}

func TestDeployKhalaAggregatesWorkersAndStopsNodeSequence(t *testing.T) {
	originalLocal, originalServer := localCommandFn, serverExecFn
	originalCorePool, originalSleep := setDefaultCorePoolFn, sleepFn
	t.Cleanup(func() {
		localCommandFn, serverExecFn = originalLocal, originalServer
		setDefaultCorePoolFn, sleepFn = originalCorePool, originalSleep
	})
	localCommandFn = func(string) (string, error) { return "", nil }
	sleepFn = func(time.Duration) { t.Fatal("slept after worker deployment failure") }
	setDefaultCorePoolFn = func(string) error {
		t.Fatal("configured core pool after worker deployment failure")
		return nil
	}

	var mu sync.Mutex
	calls := map[string]int{}
	serverExecFn = func(node, command string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls[node]++
		return "", fmt.Errorf("injected %s", node)
	}
	mode, _ := resolveExperimentMode(ModeNexusGo, 4, 256*1024)
	err := DeployKhala(WorkerNodeSetup{
		WorkerNodes:  []string{"worker-a", "worker-b"},
		StorageNodes: []string{"storage-a", "storage-b"},
	}, "", "go", mode, false)
	if err == nil || !strings.Contains(err.Error(), "worker-a") || !strings.Contains(err.Error(), "worker-b") {
		t.Fatalf("worker errors not aggregated: %v", err)
	}
	for node, count := range calls {
		if count != 1 {
			t.Fatalf("%s executed %d commands after its first failure", node, count)
		}
	}
}

func TestDeployKhalaMinIOFailurePreventsRemoteWork(t *testing.T) {
	originalLocal, originalServer := localCommandFn, serverExecFn
	t.Cleanup(func() { localCommandFn, serverExecFn = originalLocal, originalServer })
	localCommandFn = func(string) (string, error) { return "failure output", errors.New("injected") }
	serverExecFn = func(string, string) (string, error) {
		t.Fatal("remote command executed after MinIO preparation failure")
		return "", nil
	}
	mode, _ := resolveExperimentMode(ModeNexusGo, 4, 256*1024)
	if err := DeployKhala(WorkerNodeSetup{WorkerNodes: []string{"worker"}, StorageNodes: []string{"storage"}}, "", "go", mode, false); err == nil {
		t.Fatal("MinIO preparation failure suppressed")
	}
}

func TestDeployRDMAAggregatesAndStopsNodeSequence(t *testing.T) {
	originalServer := serverExecFn
	t.Cleanup(func() { serverExecFn = originalServer })
	var mu sync.Mutex
	calls := map[string]int{}
	serverExecFn = func(node, command string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls[node]++
		return "", fmt.Errorf("injected %s", node)
	}
	err := DeployRDMAStorage(WorkerNodeSetup{StorageNodes: []string{"storage-a", "storage-b"}})
	if err == nil || !strings.Contains(err.Error(), "storage-a") || !strings.Contains(err.Error(), "storage-b") {
		t.Fatalf("RDMA errors not aggregated: %v", err)
	}
	for node, count := range calls {
		if count != 1 {
			t.Fatalf("%s executed %d commands after its first failure", node, count)
		}
	}
}

func TestCreateSnapshotsPropagatesAllWorkerFailures(t *testing.T) {
	originalCreate := createSnapshotsNodeFn
	t.Cleanup(func() { createSnapshotsNodeFn = originalCreate })
	createSnapshotsNodeFn = func(node string, workloads []string) error {
		return fmt.Errorf("snapshot failure on %s", node)
	}
	mode, _ := resolveExperimentMode(ModeNexusGo, 4, 256*1024)
	err := CreateSnapshots(WorkerNodeSetup{WorkerNodes: []string{"worker-a", "worker-b"}}, mode)
	if err == nil || !strings.Contains(err.Error(), "worker-a") || !strings.Contains(err.Error(), "worker-b") {
		t.Fatalf("snapshot errors not aggregated: %v", err)
	}
}
