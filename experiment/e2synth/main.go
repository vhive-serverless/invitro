package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/vhive-serverless/loader/experiment/eval"
)

type options struct {
	common                                                   eval.Config
	e1Summary, reference, modes, payloads, clusterID         string
	workerCores, replicas, repetitions                       int
	sloMultiplier, failureThreshold, ceilingMultiplier       string
	warmupMinutes, steps, minutesPerStep, measurementMinutes int
	smoke                                                    bool
}

const workerPodReserve = 40

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "plan" && os.Args[1] != "calibrate" && os.Args[1] != "collect" && os.Args[1] != "smoke") {
		fail("usage: go run ./experiment/e2synth plan|calibrate|collect|smoke [flags]")
	}
	action := os.Args[1]
	fs := flag.NewFlagSet("e2 "+action, flag.ContinueOnError)
	o := defaultOptions(action)
	eval.AddFlags(fs, &o.common)
	fs.StringVar(&o.e1Summary, "e1-summary", "", "E1 B0 unloaded-average CSV")
	fs.StringVar(&o.reference, "reference", "", "frozen B0 RPS reference CSV")
	fs.StringVar(&o.modes, "modes", o.modes, "comma-separated E2-Synth modes")
	fs.StringVar(&o.payloads, "payloads", o.payloads, "comma-separated payload bytes")
	fs.StringVar(&o.clusterID, "cluster-id", "", "stable cluster identity recorded in every cell")
	fs.IntVar(&o.workerCores, "worker-cores", 0, "override discovered worker allocatable cores")
	fs.StringVar(&o.sloMultiplier, "slo-multiplier", "5", "B0 p99 SLO multiplier")
	fs.StringVar(&o.failureThreshold, "failure-threshold", "0.05", "request failure threshold")
	fs.StringVar(&o.ceilingMultiplier, "ceiling-multiplier", "1", "single-pass sweep ceiling multiplier")
	fs.IntVar(&o.warmupMinutes, "warmup-minutes", 2, "warmup minutes")
	fs.IntVar(&o.steps, "steps", 20, "calibration steps")
	fs.IntVar(&o.minutesPerStep, "minutes-per-step", 1, "minutes per calibration step")
	fs.IntVar(&o.measurementMinutes, "measurement-minutes", o.measurementMinutes, "fixed-RPS measurement minutes")
	fs.IntVar(&o.replicas, "replicas", o.replicas, "requested fixed replica count")
	fs.IntVar(&o.repetitions, "repetitions", 1, "independent campaign repetitions")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fail(err.Error())
	}
	if err := run(context.Background(), action, o); err != nil {
		fail(err.Error())
	}
}

func defaultOptions(action string) options {
	o := options{common: eval.Config{Profile: eval.Profile4}, replicas: 320, repetitions: 1,
		modes:         "invm-py,invm-js,invm-go,hosttcp-go,nexus-py,nexus-js,nexus-go,nexus-rdma-py,nexus-rdma-go",
		payloads:      "4,4096,16384,65536,262144,1048576,2097152,4194304,8388608,16777216",
		sloMultiplier: "5", failureThreshold: "0.05", ceilingMultiplier: "1",
		warmupMinutes: 2, steps: 20, minutesPerStep: 1, measurementMinutes: 3}
	if action == "smoke" {
		o.smoke, o.replicas, o.measurementMinutes = true, 2, 1
	} else if action == "calibrate" {
		o.modes = "invm-py"
	}
	return o
}

func run(ctx context.Context, action string, o options) error {
	return runWithWorkerPodDiscovery(ctx, action, o, discoverWorkerPods)
}

func runWithWorkerPodDiscovery(ctx context.Context, action string, o options, discoverPods func(context.Context) (int, error)) error {
	if o.common.Profile != eval.Profile4 {
		return fmt.Errorf("E2-Synth requires profile 4-node")
	}
	if o.common.TopologyConfig == "" || o.common.ResultRoot == "" {
		return fmt.Errorf("--topology-config and --result-root are required")
	}
	setup, err := eval.LoadSetup(o.common.TopologyConfig)
	if err != nil {
		return err
	}
	if err := eval.ValidateSetup(setup, o.common.Profile); err != nil {
		return err
	}
	_, clientEndpoint, err := eval.NormalizeMinioEndpoint(o.common.MinioEndpoint)
	if err != nil {
		return err
	}
	if o.repetitions <= 0 || o.warmupMinutes <= 0 || o.measurementMinutes <= 0 || o.replicas <= 0 {
		return fmt.Errorf("repetitions, warmup, measurement duration, and replicas must be positive")
	}
	if action == "plan" {
		o.common.DryRun = true
		if o.clusterID == "" {
			o.clusterID = "plan"
		}
	} else if o.clusterID == "" {
		return fmt.Errorf("--cluster-id is required")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	adapterAction := action
	cleanupSmoke := func() {}
	if action == "smoke" || action == "plan" {
		if o.replicas != 2 || o.measurementMinutes != 1 || o.warmupMinutes != 2 {
			if action == "smoke" {
				return fmt.Errorf("E2-Synth smoke requires two replicas, two-minute warmup, and one measurement minute")
			}
		}
		o.e1Summary, o.reference, cleanupSmoke, err = makeSmokeInputs()
		if err != nil {
			return err
		}
		defer cleanupSmoke()
		adapterAction = "collect"
	}
	args := []string{adapterAction, "--profile", string(o.common.Profile), "--result-root", o.common.ResultRoot,
		"--minio-endpoint", clientEndpoint, "--cluster-id", o.clusterID, "--modes", o.modes,
		"--payloads", o.payloads, "--repetitions", strconv.Itoa(o.repetitions)}
	if action == "calibrate" {
		if o.e1Summary == "" {
			return fmt.Errorf("calibrate requires --e1-summary")
		}
		if o.workerCores == 0 {
			o.workerCores, err = discoverWorkerCores(ctx)
			if err != nil {
				return err
			}
		}
		if o.steps <= 0 || o.minutesPerStep <= 0 {
			return fmt.Errorf("calibration steps and minutes per step must be positive")
		}
		if err := validatePositiveFloat("slo-multiplier", o.sloMultiplier); err != nil {
			return err
		}
		if err := validatePositiveFloat("ceiling-multiplier", o.ceilingMultiplier); err != nil {
			return err
		}
		failure, err := strconv.ParseFloat(o.failureThreshold, 64)
		if err != nil || failure < 0 || failure >= 1 {
			return fmt.Errorf("failure-threshold must be in [0,1)")
		}
		args = append(args, "--e1-summary", o.e1Summary, "--worker-cores", strconv.Itoa(o.workerCores),
			"--slo-multiplier", o.sloMultiplier, "--failure-threshold", o.failureThreshold,
			"--ceiling-multiplier", o.ceilingMultiplier, "--warmup-minutes", strconv.Itoa(o.warmupMinutes),
			"--steps", strconv.Itoa(o.steps), "--minutes-per-step", strconv.Itoa(o.minutesPerStep))
	} else {
		if o.reference == "" || o.e1Summary == "" {
			return fmt.Errorf("collect requires --reference and --e1-summary")
		}
		args = append(args, "--reference", o.reference, "--e1-summary", o.e1Summary, "--replicas", strconv.Itoa(o.replicas),
			"--warmup-minutes", strconv.Itoa(o.warmupMinutes), "--measurement-minutes", strconv.Itoa(o.measurementMinutes))
		if o.smoke {
			args = append(args, "--smoke")
		}
	}
	if o.common.DryRun {
		args = append(args, "--dry-run")
	}
	if !o.common.DryRun && !o.smoke {
		if err := requireWorkerPodCapacity(ctx, o.replicas, discoverPods); err != nil {
			return err
		}
	}
	return (eval.Runner{DryRun: false}).Run(ctx, eval.Command{Name: filepath.Join(root, "run_e2_synth.sh"), Args: args, Dir: root})
}

func validatePositiveFloat(name, text string) error {
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || value <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}

func makeSmokeInputs() (averages, reference string, cleanup func(), err error) {
	directory, err := os.MkdirTemp("", "nexus-e2-synth-smoke-inputs-")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(directory) }
	averages = filepath.Join(directory, "non-claiming-averages.csv")
	reference = filepath.Join(directory, "non-claiming-reference.csv")
	averageRows := []string{"payload_bytes,unloaded_average_ms,n_samples"}
	referenceRows := []string{"payload_bytes,calibration_cluster,unloaded_average_ms,worker_cores,ceiling_multiplier,rbound,first_failing_step,first_failing_rps,rmax_b0,rref,status,reference_kind"}
	for _, payload := range []int{4, 4096, 16384, 65536, 262144, 1048576, 2097152, 4194304, 8388608, 16777216} {
		value := strconv.Itoa(payload)
		averageRows = append(averageRows, value+",1,1")
		referenceRows = append(referenceRows, value+",smoke,1,1,1,1,,,1,1,BOUNDARY_OBSERVED,NON_CLAIMING_SMOKE_FIXTURE")
	}
	if err := os.WriteFile(averages, []byte(strings.Join(averageRows, "\n")+"\n"), 0600); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	if err := os.WriteFile(reference, []byte(strings.Join(referenceRows, "\n")+"\n"), 0600); err != nil {
		cleanup()
		return "", "", func() {}, err
	}
	return averages, reference, cleanup, nil
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func discoverWorkerCores(ctx context.Context) (int, error) {
	if raw := os.Getenv("NEXUS_WORKER_CORES"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil && value > 0 {
			return value, nil
		}
	}
	out, err := exec.CommandContext(ctx, "kubectl", "get", "nodes", "-l", "loader-nodetype=worker", "-o", "jsonpath={range .items[*]}{.status.allocatable.cpu}{'\\n'}{end}").Output()
	if err != nil {
		return 0, fmt.Errorf("discover worker cores: %w", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 1 {
		return 0, fmt.Errorf("expected one worker CPU value, got %q", strings.TrimSpace(string(out)))
	}
	raw := lines[0]
	if strings.HasSuffix(raw, "m") {
		milli, err := strconv.Atoi(strings.TrimSuffix(raw, "m"))
		if err != nil || milli < 1000 {
			return 0, fmt.Errorf("invalid allocatable CPU %q", raw)
		}
		return milli / 1000, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid allocatable CPU %q", raw)
	}
	return value, nil
}

func requireWorkerPodCapacity(ctx context.Context, replicas int, discover func(context.Context) (int, error)) error {
	capacity, err := discover(ctx)
	if err != nil {
		return err
	}
	required := replicas + workerPodReserve
	if capacity < required {
		return fmt.Errorf("worker allocatable pod capacity %d < required %d (%d replicas + %d reserve)", capacity, required, replicas, workerPodReserve)
	}
	return nil
}

func discoverWorkerPods(ctx context.Context) (int, error) {
	out, err := exec.CommandContext(ctx, "kubectl", "get", "nodes", "-l", "loader-nodetype=worker", "-o", "jsonpath={range .items[*]}{.status.allocatable.pods}{'\\n'}{end}").Output()
	if err != nil {
		return 0, fmt.Errorf("discover worker pod capacity: %w", err)
	}
	lines := strings.Fields(string(out))
	if len(lines) != 1 {
		return 0, fmt.Errorf("expected one worker allocatable pod value, got %q", strings.TrimSpace(string(out)))
	}
	value, err := strconv.Atoi(lines[0])
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("invalid worker allocatable pod value %q", lines[0])
	}
	return value, nil
}

func fail(message string) { fmt.Fprintln(os.Stderr, "e2-synth:", message); os.Exit(2) }
