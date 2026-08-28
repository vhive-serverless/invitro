package main

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveExperimentModes(t *testing.T) {
	tests := []struct {
		name, backend  string
		sdk, rpc, rdma bool
		workloads      []string
	}{
		{ModeInVMPy, "none", false, false, false, []string{"pyaesserve", "mapper", "reducer"}},
		{ModeInVMGo, "none", false, false, false, []string{"gohelloworld"}},
		{ModeInVMJS, "none", false, false, false, []string{"jshelloworld"}},
		{ModeNexusPy, "shmem", true, true, false, []string{"pyaesserve", "mapper", "reducer"}},
		{ModeNexusGo, "shmem", true, true, false, []string{"gopyaesserve", "gomapper", "goreducer"}},
		{ModeNexusJS, "shmem", true, true, false, []string{"jshelloworld"}},
		{ModeHostTCPGo, "hosttcp", true, true, false, []string{"gopyaesserve", "gomapper", "goreducer"}},
		{ModeNexusRDMA, "rdma", true, true, true, []string{"gopyaesserve", "gomapper", "goreducer"}},
		{ModeNexusRDMAPy, "rdma", true, true, true, []string{"pyaesserve", "mapper", "reducer"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode, err := resolveExperimentMode(test.name, 4_190_208, 256*1024)
			if err != nil {
				t.Fatal(err)
			}
			if mode.BackendTransport != test.backend ||
				mode.SetNexusSDK != test.sdk || mode.SetNexusRPC != test.rpc || mode.WithRDMA != test.rdma {
				t.Fatalf("unexpected mode: %+v", mode)
			}
			if strings.Join(mode.Workloads, ",") != strings.Join(test.workloads, ",") {
				t.Fatalf("workloads = %v, want %v", mode.Workloads, test.workloads)
			}
		})
	}
}

func TestResolveExplicitEvaluationWorkloads(t *testing.T) {
	for _, name := range []string{ModeInVMPy, ModeNexusPy, ModeNexusRDMAPy} {
		mode, err := resolveExperimentMode(name, 4_190_208, 256*1024, pythonEvaluationWorkloads...)
		if err != nil {
			t.Fatalf("%s full Python workload set: %v", name, err)
		}
		if strings.Join(mode.Workloads, ",") != strings.Join(pythonEvaluationWorkloads, ",") {
			t.Fatalf("%s workloads = %v", name, mode.Workloads)
		}
	}
	for _, name := range []string{ModeInVMGo, ModeHostTCPGo, ModeNexusGo, ModeInVMJS, ModeNexusJS} {
		mode, err := resolveExperimentMode(name, 4_190_208, 256*1024, "helloworld")
		if err != nil || len(mode.Workloads) != 1 || mode.Workloads[0] == "" {
			t.Fatalf("%s HelloWorld resolution = %+v, %v", name, mode, err)
		}
	}
	for _, name := range []string{ModeInVMGo, ModeHostTCPGo, ModeNexusGo, ModeInVMJS, ModeNexusJS} {
		if _, err := resolveExperimentMode(name, 4_190_208, 256*1024, "cnnserve"); err == nil {
			t.Fatalf("%s accepted an unimplemented cnnserve workload", name)
		}
	}
}

func TestResolveExperimentModeRejectsAmbiguity(t *testing.T) {
	for _, name := range []string{"", "nexus", "async", "unknown-mode"} {
		if _, err := resolveExperimentMode(name, 4_190_208, 256*1024); err == nil {
			t.Fatalf("resolveExperimentMode(%q) succeeded", name)
		}
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 0, 256*1024); err == nil {
		t.Fatal("zero slots accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 1, 256*1024); err == nil {
		t.Fatal("ring smaller than the I/O quantum accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, 64, 256*1024); err == nil {
		t.Fatal("ring larger than the backing accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusGo, int(^uint(0)>>1), int(^uint(0)>>1)); err == nil {
		t.Fatal("overflow-sized shared-memory layout accepted")
	}
	if _, err := resolveExperimentMode(ModeNexusRDMA, 4_190_208, 256*1024); err != nil {
		t.Fatalf("RDMA incorrectly subjected to shared-memory reservation: %v", err)
	}
}

func TestResolveCleanupModeValidatesOnlyTeardownIdentity(t *testing.T) {
	for _, test := range []struct {
		name     string
		withRDMA bool
	}{
		{ModeInVMPy, false},
		{ModeInVMGo, false},
		{ModeInVMJS, false},
		{ModeNexusPy, false},
		{ModeNexusGo, false},
		{ModeNexusJS, false},
		{ModeHostTCPGo, false},
		{ModeHostTCPPy, false},
		{ModeNexusRDMA, true},
		{ModeNexusRDMAPy, true},
	} {
		mode, err := resolveCleanupMode(test.name)
		if err != nil {
			t.Fatalf("resolveCleanupMode(%q): %v", test.name, err)
		}
		if mode.WithRDMA != test.withRDMA {
			t.Fatalf("resolveCleanupMode(%q).WithRDMA = %t, want %t", test.name, mode.WithRDMA, test.withRDMA)
		}
		if mode.ShmemRingBytes != 0 || mode.ShmemIOQuantum != 0 || mode.BackendTransport != "" || len(mode.Workloads) != 0 {
			t.Fatalf("cleanup resolved irrelevant deployment state for %q: %+v", test.name, mode)
		}
	}
	if _, err := resolveCleanupMode("historical-unknown-mode"); err == nil {
		t.Fatal("unknown cleanup mode accepted")
	}
}

func TestRunCommandCleanupBypassesHistoricalShmemValidation(t *testing.T) {
	originalCommand, originalMode := *Command, *Mode
	originalCorePoolPolicy := *CorePoolPolicy
	originalImplementation, originalRing, originalQuantum := *Implementation, *ShmemRingBytes, *ShmemIOQuantum
	originalDryRun, originalRemoveSnapshots := *DryRun, *RemoveSnapshots
	originalGetWorkers, originalClean := getWorkerNodesFn, cleanKhalaFn
	t.Cleanup(func() {
		*Command, *Mode = originalCommand, originalMode
		*CorePoolPolicy = originalCorePoolPolicy
		*Implementation, *ShmemRingBytes, *ShmemIOQuantum = originalImplementation, originalRing, originalQuantum
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
			*Mode, *ShmemRingBytes, *ShmemIOQuantum = test.name, test.slots, test.capacity
			called := false
			cleanKhalaFn = func(got WorkerNodeSetup, removeSnapshots, withRDMA bool) error {
				called = true
				if strings.Join(got.WorkerNodes, ",") != "worker" || !removeSnapshots || withRDMA != test.wantRDMA {
					t.Fatalf("cleanup args = %+v, remove=%t, rdma=%t", got, removeSnapshots, withRDMA)
				}
				return nil
			}
			if err := runCommand(); err != nil {
				t.Fatalf("cleanup rejected irrelevant shared-memory layout: %v", err)
			}
			if !called {
				t.Fatal("cleanup was not performed")
			}
		})
	}

	*Mode, *ShmemRingBytes, *ShmemIOQuantum = "unknown", 1, int(^uint(0)>>1)
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
		ModeInVMPy:      {"pyaesserve-0", "mapper-0", "reducer-0"},
		ModeNexusPy:     {"pyaesserve-s3-rpc-shmem-0", "mapper-s3-rpc-shmem-0", "reducer-s3-rpc-shmem-0"},
		ModeNexusGo:     {"gopyaesserve-s3-rpc-shmem-0", "gomapper-s3-rpc-shmem-0", "goreducer-s3-rpc-shmem-0"},
		ModeHostTCPGo:   {"gopyaesserve-s3-rpc-hosttcp-0", "gomapper-s3-rpc-hosttcp-0", "goreducer-s3-rpc-hosttcp-0"},
		ModeHostTCPPy:   {"pyaesserve-s3-rpc-hosttcp-0", "mapper-s3-rpc-hosttcp-0", "reducer-s3-rpc-hosttcp-0"},
		ModeNexusRDMA:   {"gopyaesserve-s3-rpc-rdma-0", "gomapper-s3-rpc-rdma-0", "goreducer-s3-rpc-rdma-0"},
		ModeNexusRDMAPy: {"pyaesserve-s3-rpc-rdma-0", "mapper-s3-rpc-rdma-0", "reducer-s3-rpc-rdma-0"},
	}
	for name, want := range tests {
		mode, _ := resolveExperimentMode(name, 4_190_208, 256*1024)
		got := buildSnapshotNames(mode)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("%s snapshots = %v, want %v", name, got, want)
		}
	}
}

func TestDeploymentCommandCarriesExplicitPlacement(t *testing.T) {
	mode, _ := resolveExperimentMode(ModeNexusGo, 4_190_208, 131072)
	command := buildDeploymentCommand("", "go", mode, false)
	for _, fragment := range []string{
		"--backend-transport=shmem",
		"--minio-endpoint=10.0.1.4:9001",
		"--set-nexus-sdk=true", "--set-nexus-rpc=true", "--with-rdma=false",
		"--shmem-ring-bytes=4190208", "--shmem-io-quantum=262144",
	} {
		if !strings.Contains(command, fragment) {
			t.Errorf("deployment command %q missing %q", command, fragment)
		}
	}
}

func TestHostTCPDeploymentCommandCarriesFrozenBoundary(t *testing.T) {
	mode, err := resolveExperimentMode(ModeHostTCPGo, 4_190_208, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	command := buildDeploymentCommand("", "go", mode, false)
	for _, fragment := range []string{
		"--backend-transport=hosttcp",
		"--minio-endpoint=10.0.1.4:9001",
		"--set-nexus-sdk=true", "--set-nexus-rpc=true", "--with-rdma=false",
		"--shmem-ring-bytes=4190208", "--shmem-io-quantum=262144",
	} {
		if !strings.Contains(command, fragment) {
			t.Errorf("HostTCP deployment command %q missing %q", command, fragment)
		}
	}
}

func TestMinioObjectEndpointFollowsDeploymentEndpoint(t *testing.T) {
	original := *MinioEndpoint
	t.Cleanup(func() { *MinioEndpoint = original })

	for endpoint, want := range map[string]string{
		"10.0.1.9:9001":              "http://10.0.1.9:9001",
		"http://10.0.1.9:9001":       "http://10.0.1.9:9001",
		"https://minio.example:9443": "https://minio.example:9443",
	} {
		*MinioEndpoint = endpoint
		if got := minioObjectEndpointURL(); got != want {
			t.Fatalf("minioObjectEndpointURL(%q) = %q, want %q", endpoint, got, want)
		}
	}
}

func TestDryRunPlanIsModeAware(t *testing.T) {
	mode, _ := resolveExperimentMode(ModeNexusRDMA, 4_190_208, 256*1024)
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
	mode, _ := resolveExperimentMode(ModeNexusGo, 4_190_208, 256*1024)
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
	mode, _ := resolveExperimentMode(ModeNexusGo, 4_190_208, 256*1024)
	if err := DeployKhala(WorkerNodeSetup{WorkerNodes: []string{"worker"}, StorageNodes: []string{"storage"}}, "", "go", mode, false); err == nil {
		t.Fatal("MinIO preparation failure suppressed")
	}
}

func TestDeployKhalaWaitsForWorkerReadiness(t *testing.T) {
	originalLocal, originalServer := localCommandFn, serverExecFn
	originalCorePool, originalWait := setDefaultCorePoolFn, waitKnIntegrationFn
	t.Cleanup(func() {
		localCommandFn, serverExecFn = originalLocal, originalServer
		setDefaultCorePoolFn, waitKnIntegrationFn = originalCorePool, originalWait
	})

	localCommandFn = func(string) (string, error) { return "", nil }
	serverExecFn = func(string, string) (string, error) { return "", nil }
	ready := false
	waitKnIntegrationFn = func(node string, timeout time.Duration) error {
		if node != "worker" || timeout != knIntegrationStartTimeout {
			t.Fatalf("readiness args = %s, %s", node, timeout)
		}
		ready = true
		return nil
	}
	setDefaultCorePoolFn = func(node string) error {
		if node != "worker" || !ready {
			t.Fatalf("core pool configured before readiness: node=%s ready=%t", node, ready)
		}
		return nil
	}
	mode, _ := resolveExperimentMode(ModeInVMPy, 4_190_208, 256*1024)
	if err := DeployKhala(WorkerNodeSetup{WorkerNodes: []string{"worker"}, StorageNodes: []string{"storage"}}, "", "go", mode, false); err != nil {
		t.Fatal(err)
	}
	if !ready {
		t.Fatal("deployment returned without a readiness check")
	}
}

func TestLoggedKnIntegrationCommandRetainsExitEvidence(t *testing.T) {
	command := loggedKnIntegrationCommand("cd ~/khala && sudo ./bin/kn-integration")
	for _, want := range []string{"tmux new-session", "kn-integration.log", "kn-integration.exit", "status=$?"} {
		if !strings.Contains(command, want) {
			t.Fatalf("logged command missing %q: %s", want, command)
		}
	}
	if strings.Contains(command, "send-keys") {
		t.Fatalf("logged command detached from child lifecycle: %s", command)
	}
}

func TestKnIntegrationReadinessReportsDetachedChildExit(t *testing.T) {
	originalServer, originalDial, originalSleep := serverExecFn, dialKnIntegrationFn, sleepFn
	t.Cleanup(func() {
		serverExecFn, dialKnIntegrationFn, sleepFn = originalServer, originalDial, originalSleep
	})
	dialKnIntegrationFn = func(string, time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
	serverExecFn = func(_ string, command string) (string, error) {
		if strings.Contains(command, "tmux_alive") {
			return "tmux_alive=false\nexit_status=2\npanic: injected", nil
		}
		return "exit_status=2", errors.New("child exited")
	}
	sleepFn = func(time.Duration) { t.Fatal("slept after child exit was observed") }
	err := waitForKnIntegration("worker", time.Second)
	if err == nil || !strings.Contains(err.Error(), "exited before readiness") || !strings.Contains(err.Error(), "panic: injected") {
		t.Fatalf("readiness error = %v", err)
	}
}

func TestKnIntegrationReadinessDoesNotMislabelProbeFailureAsChildExit(t *testing.T) {
	originalServer, originalDial, originalSleep := serverExecFn, dialKnIntegrationFn, sleepFn
	t.Cleanup(func() {
		serverExecFn, dialKnIntegrationFn, sleepFn = originalServer, originalDial, originalSleep
	})
	dialKnIntegrationFn = func(string, time.Duration) (net.Conn, error) {
		return nil, errors.New("connection refused")
	}
	serverExecFn = func(string, string) (string, error) { return "ssh unavailable", errors.New("transport failure") }
	sleepFn = func(time.Duration) { t.Fatal("slept after zero timeout") }
	err := waitForKnIntegration("worker", 0)
	if err == nil || !strings.Contains(err.Error(), "readiness timed out") || strings.Contains(err.Error(), "exited before readiness") {
		t.Fatalf("readiness error = %v", err)
	}
}

func TestKnIntegrationReadinessWaitsForHardwareManager(t *testing.T) {
	originalServer, originalDial, originalSleep := serverExecFn, dialKnIntegrationFn, sleepFn
	t.Cleanup(func() {
		serverExecFn, dialKnIntegrationFn, sleepFn = originalServer, originalDial, originalSleep
	})
	serverExecFn = func(string, string) (string, error) { return "", nil }
	calls := map[string]int{}
	dialKnIntegrationFn = func(address string, _ time.Duration) (net.Conn, error) {
		calls[address]++
		if strings.HasSuffix(address, ":8002") && calls[address] == 1 {
			return nil, errors.New("hardware manager is still starting")
		}
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	}
	sleeps := 0
	sleepFn = func(time.Duration) { sleeps++ }
	if err := waitForKnIntegration("worker", time.Second); err != nil {
		t.Fatal(err)
	}
	if calls["worker:8000"] != 2 || calls["worker:8002"] != 2 || sleeps != 1 {
		t.Fatalf("readiness calls=%v sleeps=%d", calls, sleeps)
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

func TestCleanKhalaTreatsUnavailableDestroyAllAsBestEffort(t *testing.T) {
	originalServer, originalLocal, originalDestroy := serverExecFn, cleanupLocalCommandFn, destroyAllFn
	t.Cleanup(func() {
		serverExecFn, cleanupLocalCommandFn, destroyAllFn = originalServer, originalLocal, originalDestroy
	})
	destroyAllFn = func(string) error { return errors.New("injected unavailable RPC") }
	cleanupLocalCommandFn = func(string) (string, error) { return "", nil }
	var commands []string
	serverExecFn = func(_ string, command string) (string, error) {
		commands = append(commands, command)
		return "", nil
	}
	if err := CleanKhala(WorkerNodeSetup{WorkerNodes: []string{"worker"}}, true, false); err != nil {
		t.Fatalf("unavailable DestroyAll made forced cleanup fail: %v", err)
	}
	if !strings.Contains(strings.Join(commands, "\n"), "snapshots/*.snapshot") {
		t.Fatalf("forced snapshot cleanup was not attempted: %v", commands)
	}
	for _, forbidden := range []string{"rollout restart", "cleanup_minio", "redeploy_minio"} {
		if strings.Contains(strings.Join(commands, "\n"), forbidden) {
			t.Fatalf("cleanup performed forbidden recovery %q: %v", forbidden, commands)
		}
	}
}

func TestCleanKhalaKeepsForcedSnapshotCleanupFatal(t *testing.T) {
	originalServer, originalLocal, originalDestroy := serverExecFn, cleanupLocalCommandFn, destroyAllFn
	t.Cleanup(func() {
		serverExecFn, cleanupLocalCommandFn, destroyAllFn = originalServer, originalLocal, originalDestroy
	})
	destroyAllFn = func(string) error { return errors.New("injected unavailable RPC") }
	cleanupLocalCommandFn = func(string) (string, error) { return "", nil }
	serverExecFn = func(_ string, command string) (string, error) {
		if strings.Contains(command, "snapshots/*.snapshot") {
			return "snapshot permission denied", errors.New("injected snapshot failure")
		}
		return "", nil
	}
	err := CleanKhala(WorkerNodeSetup{WorkerNodes: []string{"worker"}}, true, false)
	if err == nil || !strings.Contains(err.Error(), "snapshot failure") {
		t.Fatalf("forced snapshot cleanup failure was suppressed: %v", err)
	}
}

func TestCreateSnapshotsPropagatesAllWorkerFailures(t *testing.T) {
	originalCreate := createSnapshotsNodeFn
	t.Cleanup(func() { createSnapshotsNodeFn = originalCreate })
	createSnapshotsNodeFn = func(node string, workloads []string) error {
		return fmt.Errorf("snapshot failure on %s", node)
	}
	mode, _ := resolveExperimentMode(ModeNexusGo, 4_190_208, 256*1024)
	err := CreateSnapshots(WorkerNodeSetup{WorkerNodes: []string{"worker-a", "worker-b"}}, mode)
	if err == nil || !strings.Contains(err.Error(), "worker-a") || !strings.Contains(err.Error(), "worker-b") {
		t.Fatalf("snapshot errors not aggregated: %v", err)
	}
}
