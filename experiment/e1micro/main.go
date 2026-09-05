package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vhive-serverless/loader/experiment/eval"
)

var identities = []string{"microbench-grpc", "microbench-grpc-minio", "microbench-grpc-s3", "microbench-python-loop", "microbench-vsock", "microbench-vsock-minio", "microbench-vsock-s3", "microbench-tcp", "idle-vm"}

type options struct {
	profile                                       eval.Profile
	topology, minio, result, manifest, ids, runID string
	latency, memory, warm, settle, window         int
	smoke, dry                                    bool
}

func main() {
	fs := flag.NewFlagSet("e1micro", flag.ContinueOnError)
	o := options{profile: eval.Profile4, ids: strings.Join(identities, ","), latency: 20, memory: 20, warm: 1, settle: 60, window: 5}
	fs.Var((*profileValue)(&o.profile), "profile", "topology profile")
	fs.StringVar(&o.topology, "topology-config", "", "topology JSON")
	fs.StringVar(&o.minio, "minio-endpoint", "", "MinIO endpoint")
	fs.StringVar(&o.result, "result-root", "", "result root")
	fs.StringVar(&o.manifest, "campaign-manifest", "", "campaign manifest")
	fs.StringVar(&o.ids, "identities", o.ids, "comma-separated identities")
	fs.StringVar(&o.runID, "run-id", "", "run/snapshot identity")
	fs.IntVar(&o.latency, "latency-iterations", 20, "latency iterations")
	fs.IntVar(&o.memory, "memory-iterations", 20, "memory iterations")
	fs.IntVar(&o.warm, "warm-invocations", 1, "warm invocations")
	fs.IntVar(&o.settle, "snapshot-settle-seconds", 60, "snapshot settle seconds")
	fs.IntVar(&o.window, "memory-window-seconds", 5, "memory window seconds")
	fs.BoolVar(&o.smoke, "smoke", false, "smoke counts")
	fs.BoolVar(&o.dry, "dry-run", false, "dry run")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if err := run(context.Background(), o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

type profileValue eval.Profile

func (p *profileValue) String() string     { return string(*p) }
func (p *profileValue) Set(v string) error { *p = profileValue(v); return nil }

func run(ctx context.Context, o options) error {
	if o.profile != eval.Profile4 {
		return fmt.Errorf("E1-Microbench requires profile 4-node")
	}
	if o.topology == "" || o.minio == "" || o.result == "" {
		return fmt.Errorf("--topology-config, --minio-endpoint, and --result-root are required")
	}
	if err := eval.ValidateResultRoot(o.result); err != nil {
		return err
	}
	setup, err := eval.LoadSetup(o.topology)
	if err != nil {
		return err
	}
	if err = eval.ValidateSetup(setup, o.profile); err != nil {
		return err
	}
	if len(setup.LabeledIPs("loader-nodetype=worker")) != 1 {
		return fmt.Errorf("E1-Microbench requires exactly one worker")
	}
	selected, err := selectIDs(o.ids)
	if err != nil {
		return err
	}
	if o.warm != 1 || o.latency <= 0 || o.memory <= 0 || o.settle <= 0 || o.window <= 0 {
		return fmt.Errorf("counts/windows must be positive and warm-invocations must equal 1")
	}
	if o.manifest != "" {
		if _, err = eval.RequireCampaign(o.manifest); err != nil {
			return err
		}
	}
	if o.smoke {
		o.latency, o.memory = 1, 1
	}
	restore := len(selected) * o.latency
	inv := 7 * o.latency
	mem := len(selected) * o.memory
	fmt.Printf("PLAN experiment=e1-microbench profile=%s identities=%s latency=%d memory=%d warm=%d expected_restore=%d expected_cold=%d expected_warm=%d expected_memory=%d smoke=%t\n", o.profile, strings.Join(selected, ","), o.latency, o.memory, o.warm, restore, inv, inv, mem, o.smoke)
	if o.dry {
		return nil
	}
	if _, err = os.Stat(o.result); err == nil {
		return fmt.Errorf("refusing to overwrite result root %s", o.result)
	} else if !os.IsNotExist(err) {
		return err
	}
	worker, err := setup.URLForIP(setup.LabeledIPs("loader-nodetype=worker")[0])
	if err != nil {
		return err
	}
	if err = eval.RemoteAbsent(ctx, worker, o.result); err != nil {
		return err
	}
	home, err := eval.RemoteHome(worker)
	if err != nil {
		return err
	}
	return dispatch(ctx, worker, home, o, selected)
}

func selectIDs(text string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, raw := range strings.Split(text, ",") {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			return nil, fmt.Errorf("identities must be nonempty and unique")
		}
		seen[id] = true
		found := false
		for _, known := range identities {
			if known == id {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown identity %q", id)
		}
		out = append(out, id)
	}
	if len(out) != len(identities) {
		return nil, fmt.Errorf("E1-Microbench requires all nine identities")
	}
	return out, nil
}

func dispatch(ctx context.Context, worker, home string, o options, ids []string) error {
	if err := os.MkdirAll(filepath.Dir(o.result), 0o755); err != nil {
		return err
	}
	logPath := o.result + "-dispatch.log"
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	args := []string{"env", "--chdir=" + home + "/khala", "NEXUS_MINIO_URL=" + o.minio, "./experiment-script/e1-microbench/run.sh", "--orchestrator-address=localhost:8000", "--config=configs/vm_orchestrator_e1_microbench.json", "--result-root=" + o.result, "--minio-endpoint=" + o.minio, "--measurement=both", "--identities=" + strings.Join(ids, ","), "--latency-iterations=" + strconv.Itoa(o.latency), "--memory-iterations=" + strconv.Itoa(o.memory), "--warm-invocations=1", "--snapshot-settle-seconds=" + strconv.Itoa(o.settle), "--memory-window-seconds=" + strconv.Itoa(o.window)}
	if o.runID != "" {
		args = append(args, "--run-id="+o.runID)
	}
	remoteErr := runRemote(ctx, worker, logFile, args...)
	copyErr := eval.CopyRemoteTree(ctx, worker, filepath.Clean(o.result), logFile)
	if copyErr == nil {
		copyErr = verifyTerminalResults(o)
	}
	return errors.Join(remoteErr, copyErr)
}
func runRemote(ctx context.Context, target string, out io.Writer, args ...string) error {
	cmd, err := eval.SSHCommand(ctx, target, args...)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = out, out
	return cmd.Run()
}

func verifyTerminalResults(o options) error {
	manifestBytes, err := os.ReadFile(filepath.Join(o.result, "manifest.json"))
	if err != nil {
		return err
	}
	var manifest struct {
		Status            string   `json:"status"`
		Measurement       string   `json:"measurement"`
		Identities        []string `json:"identities"`
		LatencyIterations int      `json:"latency_iterations"`
		MemoryIterations  int      `json:"memory_iterations"`
		WarmInvocations   int      `json:"warm_invocations"`
	}
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		return err
	}
	if manifest.Status != "complete" || manifest.Measurement != "both" || manifest.LatencyIterations != o.latency || manifest.MemoryIterations != o.memory || manifest.WarmInvocations != 1 || len(manifest.Identities) != 9 {
		return fmt.Errorf("terminal manifest does not match E1-Microbench contract")
	}
	counts := map[string]int{}
	if err = scanJSONL(filepath.Join(o.result, "latency.jsonl"), func(row map[string]any) error {
		if row["status"] == "error" {
			return fmt.Errorf("latency error row")
		}
		if phase, ok := row["phase"].(string); ok && row["status"] != "NA" {
			counts[phase]++
		}
		return nil
	}); err != nil {
		return err
	}
	memoryIterations := map[string]bool{}
	if err = scanJSONL(filepath.Join(o.result, "memory.jsonl"), func(row map[string]any) error {
		if row["status"] == "error" {
			return fmt.Errorf("memory error row")
		}
		identity, iok := row["identity"].(string)
		iteration, nok := row["iteration"].(float64)
		if iok && nok {
			memoryIterations[fmt.Sprintf("%s:%d", identity, int(iteration))] = true
		}
		return nil
	}); err != nil {
		return err
	}
	wantRestore, wantInvoke, wantMemory := 9*o.latency, 7*o.latency, 9*o.memory
	if counts["restore"] != wantRestore || counts["readiness"] != wantRestore || counts["cold"] != wantInvoke || counts["warm"] != wantInvoke || len(memoryIterations) != wantMemory {
		return fmt.Errorf("incomplete terminal results: restore=%d readiness=%d cold=%d warm=%d memory=%d", counts["restore"], counts["readiness"], counts["cold"], counts["warm"], len(memoryIterations))
	}
	return nil
}

func scanJSONL(path string, visit func(map[string]any) error) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var row map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return err
		}
		if err := visit(row); err != nil {
			return err
		}
	}
	return scanner.Err()
}
