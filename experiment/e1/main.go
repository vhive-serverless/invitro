package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vhive-serverless/loader/experiment/eval"
)

var realWorkloads = []string{"helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving", "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer"}
var syntheticModes = []string{"invm-py", "invm-js", "invm-go", "hosttcp-go", "nexus-py", "nexus-js", "nexus-go"}
var realModes = []string{"invm-py", "nexus-sdk-py", "nexus-py", "nexus-rdma-py"}
var syntheticPayloads = []string{"4", "4096", "16384", "65536", "262144", "1048576", "2097152", "4194304", "8388608", "16777216"}

type options struct {
	suite, modes, workloads, payloads          string
	profile, topology, minio, campaign, result string
	repetitions, latency, memory, warm         int
	dryRun, smoke                              bool
}

func main() {
	o := options{profile: string(eval.Profile4), repetitions: 1, latency: 20, memory: 20, warm: 5}
	flag.StringVar(&o.suite, "suite", "", "real or synthetic")
	flag.StringVar(&o.profile, "profile", o.profile, "4-node, 10-node, 14-node, or 18-node")
	flag.StringVar(&o.topology, "topology-config", "", "topology JSON")
	flag.StringVar(&o.minio, "minio-endpoint", "", "MinIO URL")
	flag.StringVar(&o.modes, "modes", "", "comma-separated modes")
	flag.StringVar(&o.workloads, "workloads", "", "comma-separated workloads")
	flag.StringVar(&o.payloads, "payloads", "", "comma-separated synthetic payload sizes")
	flag.IntVar(&o.repetitions, "repetitions", 1, "campaign repetitions")
	flag.IntVar(&o.latency, "latency-iterations", 20, "latency iterations")
	flag.IntVar(&o.memory, "memory-iterations", 20, "memory iterations")
	flag.IntVar(&o.warm, "warm-invocations", 5, "warm invocations")
	flag.StringVar(&o.campaign, "campaign-manifest", "", "campaign manifest")
	flag.StringVar(&o.result, "result-root", "", "result root")
	flag.BoolVar(&o.dryRun, "dry-run", false, "print plan only")
	flag.BoolVar(&o.smoke, "smoke", false, "bounded smoke matrix")
	flag.Parse()
	plan, err := makePlan(o)
	if err != nil {
		fail(err)
	}
	for _, cell := range plan {
		fmt.Printf("CELL id=%s mode=%s workload=%s payload=%s repetition=%d\n", cell.ID, cell.Mode, cell.Workload, cell.Payload, cell.Repetition)
	}
	fmt.Printf("PLAN suite=%s profile=%s cells=%d smoke=%t\n", o.suite, o.profile, len(plan), o.smoke)
	if o.dryRun {
		return
	}
	if err := runLive(context.Background(), o); err != nil {
		fail(err)
	}
}

type cell struct {
	ID, Mode, Workload, Payload string
	Repetition                  int
}

func makePlan(o options) ([]cell, error) {
	if o.suite != "real" && o.suite != "synthetic" {
		return nil, fmt.Errorf("--suite must be real or synthetic")
	}
	if o.repetitions <= 0 || o.latency <= 0 || o.memory <= 0 || o.warm <= 0 {
		return nil, fmt.Errorf("E1 requires positive repetition, latency, memory, and warm counts")
	}
	if _, err := eval.ExpectedCounts(eval.Profile(o.profile)); err != nil {
		return nil, err
	}
	if o.topology == "" || o.minio == "" || o.result == "" {
		return nil, fmt.Errorf("--topology-config, --minio-endpoint, and --result-root are required")
	}
	if _, _, err := eval.NormalizeMinioEndpoint(o.minio); err != nil {
		return nil, err
	}
	setup, err := eval.LoadSetup(o.topology)
	if err != nil {
		return nil, err
	}
	if err = eval.ValidateSetup(setup, eval.Profile(o.profile)); err != nil {
		return nil, err
	}
	if _, err = workerSSHAddress(setup); err != nil {
		return nil, err
	}
	modes := split(o.modes)
	workloads := split(o.workloads)
	payloads := split(o.payloads)
	if o.smoke {
		return smokePlan(modes, payloads)
	}
	if o.suite == "real" {
		if err := validateSelection("mode", modes, realModes); err != nil {
			return nil, err
		}
		if err := validateSelection("workload", workloads, realWorkloads); err != nil {
			return nil, err
		}
		if len(payloads) != 0 {
			return nil, fmt.Errorf("real suite does not accept payloads")
		}
		return cells("e1-real", o.profile, modes, workloads, nil, o.repetitions), nil
	}
	if err := validateSelection("mode", modes, syntheticModes); err != nil {
		return nil, err
	}
	if len(workloads) != 0 {
		return nil, fmt.Errorf("synthetic suite does not accept workloads")
	}
	if err := validatePayloads(payloads); err != nil {
		return nil, err
	}
	return cells("e1-synthetic", o.profile, modes, nil, payloads, o.repetitions), nil
}

func runLive(ctx context.Context, o options) error {
	if err := eval.ValidateResultRoot(o.result); err != nil {
		return err
	}
	if _, err := os.Stat(o.result); err == nil {
		return fmt.Errorf("refusing to overwrite result root %s", o.result)
	} else if !os.IsNotExist(err) {
		return err
	}
	setup, err := eval.LoadSetup(o.topology)
	if err != nil {
		return err
	}
	worker, err := workerSSHAddress(setup)
	if err != nil {
		return err
	}
	_, baseURL, err := eval.NormalizeMinioEndpoint(o.minio)
	if err != nil {
		return err
	}
	heads, err := eval.ResolveEvaluationHeads(ctx, o.campaign, o.smoke, setup)
	if err != nil {
		return err
	}
	if err := eval.RemoteAbsent(ctx, worker, o.result); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(o.result), 0755); err != nil {
		return err
	}
	logPath := o.result + "-dispatch.log"
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	experiment := "e1-" + o.suite
	remoteHome, err := eval.RemoteHome(worker)
	if err != nil {
		return err
	}
	args := buildRemoteArgs(o, heads, remoteHome, baseURL)
	if o.smoke {
		args = append(args, "--smoke")
	}
	fmt.Printf("ACQUISITION_START experiment=%s worker=%s result=%s log=%s\n", experiment, worker, o.result, logPath)
	remote, err := eval.SSHCommand(ctx, worker, args...)
	if err != nil {
		return err
	}
	remote.Stdout, remote.Stderr = logFile, logFile
	remoteErr := remote.Run()
	copyErr := eval.CopyRemoteTree(ctx, worker, filepath.Clean(o.result), logFile)
	if copyErr == nil {
		copyErr = verifyTerminalResults(o)
	}
	if remoteErr != nil || copyErr != nil {
		var runErr error
		if remoteErr != nil {
			runErr = fmt.Errorf("worker E1 command: %w", remoteErr)
		}
		return errors.Join(runErr, copyErr)
	}
	fmt.Printf("ACQUISITION_COMPLETE experiment=%s result=%s\n", experiment, o.result)
	return nil
}

func buildRemoteArgs(o options, heads eval.EvaluationHeads, remoteHome, baseURL string) []string {
	experiment := "e1-" + o.suite
	args := []string{"env", "NEXUS_MINIO_URL=http://" + baseURL, "NEXUS_REPETITIONS_TOTAL=" + fmt.Sprint(o.repetitions),
		"EVAL_KHALA_HEAD=" + heads.Khala, "EVAL_KHALA_BRANCH=" + eval.KhalaBranch,
		"EVAL_FIRECRACKER_HEAD=" + heads.Firecracker, "EVAL_FIRECRACKER_BRANCH=" + eval.FirecrackerBranch,
		"EVAL_KERNEL_SHA256=" + eval.KernelSHA256,
		"EVAL_RDMA_DEMO_HEAD=" + heads.RDMA, "EVAL_RDMA_DEMO_BRANCH=" + eval.RDMABranch,
		"EVAL_INVITRO_HEAD=" + heads.InVitro, "EVAL_INVITRO_BRANCH=" + eval.InVitroBranch,
		"EVAL_EAGER_BAR_PRETOUCH=true", "bash", remoteHome + "/khala/experiment-script/real-workload/run_nexus_evaluation.sh",
		"--experiment", experiment, "--result-root", filepath.Clean(o.result), "--repetitions", fmt.Sprint(o.repetitions),
		"--modes", o.modes,
		"--latency-iterations", fmt.Sprint(o.latency), "--memory-iterations", fmt.Sprint(o.memory),
		"--warm-invocations", fmt.Sprint(o.warm)}
	if o.workloads != "" {
		args = append(args, "--workloads", o.workloads)
	}
	if o.payloads != "" {
		args = append(args, "--payloads", o.payloads)
	}
	return args
}

func verifyTerminalResults(o options) error {
	expected := map[string]int{}
	perRepetition := len(split(o.modes)) * (len(split(o.workloads)) + len(split(o.payloads)))
	for repetition := 0; repetition < o.repetitions; repetition++ {
		expected[filepath.Join(o.result, fmt.Sprintf("rep-%d", repetition), "manifest.txt")] = perRepetition
	}
	if o.smoke {
		expected = map[string]int{
			filepath.Join(o.result, "smoke", "rep-0", "4b", "manifest.txt"):    7,
			filepath.Join(o.result, "smoke", "rep-0", "16mib", "manifest.txt"): 4,
		}
	}
	for path, cells := range expected {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if !strings.Contains(text, "exit_status=0\n") || !strings.Contains(text, fmt.Sprintf("expected_cell_count=%d\n", cells)) {
			return fmt.Errorf("non-terminal or wrong-size E1 manifest %s", path)
		}
	}
	return nil
}

func workerSSHAddress(setup eval.Setup) (string, error) {
	workers := setup.NodeLabel["loader-nodetype=worker"]
	if len(workers) != 1 {
		return "", fmt.Errorf("E1 requires exactly one selected worker")
	}
	index, err := eval.IPIndex(workers[0])
	if err != nil || index < 1 || index > len(setup.NodeURL) {
		return "", fmt.Errorf("worker %q has no external SSH URL", workers[0])
	}
	return setup.NodeURL[index-1], nil
}

func smokePlan(modes, payloads []string) ([]cell, error) {
	if !same(payloads, syntheticPayloads) {
		return nil, fmt.Errorf("smoke requires the frozen E1 payload list")
	}
	if !same(modes, syntheticModes) {
		return nil, fmt.Errorf("smoke requires all seven E1 synthetic modes")
	}
	result := cells("e1-smoke", "4-node", modes, nil, []string{"4"}, 1)
	result = append(result, cells("e1-smoke", "4-node", []string{"hosttcp-go", "nexus-py", "nexus-js", "nexus-go"}, nil, []string{"16777216"}, 1)...)
	return result, nil
}
func cells(experiment, profile string, modes, workloads, payloads []string, repetitions int) []cell {
	result := []cell{}
	for repetition := 0; repetition < repetitions; repetition++ {
		for _, mode := range modes {
			for _, w := range workloads {
				result = append(result, cell{eval.CellID(experiment, profile, mode, w, repetition), mode, w, "", repetition})
			}
			for _, p := range payloads {
				result = append(result, cell{eval.CellID(experiment, profile, mode, "synthetic_e_0_p_"+p, repetition), mode, "", p, repetition})
			}
		}
	}
	return result
}

func validateSelection(kind string, values, allowed []string) error {
	if len(values) == 0 {
		return fmt.Errorf("at least one %s is required", kind)
	}
	seen := map[string]bool{}
	for _, value := range values {
		if seen[value] {
			return fmt.Errorf("duplicate %s %q", kind, value)
		}
		seen[value] = true
		found := false
		for _, candidate := range allowed {
			found = found || value == candidate
		}
		if !found {
			return fmt.Errorf("unsupported %s %q", kind, value)
		}
	}
	return nil
}

func validatePayloads(values []string) error {
	if len(values) == 0 {
		return fmt.Errorf("at least one payload is required")
	}
	seen := map[int]bool{}
	for _, value := range values {
		payload, err := strconv.Atoi(value)
		if err != nil || payload <= 0 || payload > 16<<20 {
			return fmt.Errorf("payload %q must be an integer in 1..16777216", value)
		}
		if seen[payload] {
			return fmt.Errorf("duplicate payload %d", payload)
		}
		seen[payload] = true
	}
	return nil
}
func split(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
func same(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
func fail(err error) { fmt.Fprintln(os.Stderr, "e1:", err); os.Exit(2) }
