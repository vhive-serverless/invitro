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
	e1Summary, reference                                     string
	workerCores, replicas, repetitions                       int
	sloMultiplier, failureThreshold, ceilingMultiplier       string
	warmupMinutes, steps, minutesPerStep, measurementMinutes int
	noRetry, smoke                                           bool
}

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "calibrate" && os.Args[1] != "collect" && os.Args[1] != "smoke") {
		fail("usage: go run ./experiment/e2 calibrate|collect|smoke [flags]")
	}
	action := os.Args[1]
	fs := flag.NewFlagSet("e2 "+action, flag.ContinueOnError)
	o := options{common: eval.Config{Profile: eval.Profile4}, replicas: 320, repetitions: 1,
		sloMultiplier: "5", failureThreshold: "0.05", ceilingMultiplier: "1",
		warmupMinutes: 2, steps: 20, minutesPerStep: 1, measurementMinutes: 3}
	if action == "smoke" {
		o.smoke, o.replicas, o.measurementMinutes = true, 2, 1
	}
	eval.AddFlags(fs, &o.common)
	fs.StringVar(&o.e1Summary, "e1-summary", "", "E1 B0 unloaded-average CSV")
	fs.StringVar(&o.reference, "reference", "", "frozen B0 RPS reference CSV")
	fs.IntVar(&o.workerCores, "worker-cores", 0, "override discovered worker allocatable cores")
	fs.StringVar(&o.sloMultiplier, "slo-multiplier", "5", "B0 p99 SLO multiplier")
	fs.StringVar(&o.failureThreshold, "failure-threshold", "0.05", "request failure threshold")
	fs.StringVar(&o.ceilingMultiplier, "ceiling-multiplier", "1", "single-pass sweep ceiling multiplier")
	fs.IntVar(&o.warmupMinutes, "warmup-minutes", 2, "warmup minutes")
	fs.IntVar(&o.steps, "steps", 20, "calibration steps")
	fs.IntVar(&o.minutesPerStep, "minutes-per-step", 1, "minutes per calibration step")
	fs.IntVar(&o.measurementMinutes, "measurement-minutes", 3, "fixed-RPS measurement minutes")
	fs.IntVar(&o.replicas, "replicas", 320, "requested fixed replica count")
	fs.IntVar(&o.repetitions, "repetitions", 1, "independent campaign repetitions")
	fs.BoolVar(&o.noRetry, "no-retry", false, "forbid acquisition-cell retry")
	if err := fs.Parse(os.Args[2:]); err != nil {
		fail(err.Error())
	}
	if err := run(context.Background(), action, o); err != nil {
		fail(err.Error())
	}
}

func run(ctx context.Context, action string, o options) error {
	if o.common.Profile != eval.Profile4 {
		return fmt.Errorf("E2 requires profile 4-node")
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
	if o.repetitions != 1 {
		return fmt.Errorf("single-pass E2 requires --repetitions 1")
	}
	if !o.common.DryRun && !o.smoke {
		if _, err := eval.RequireCampaign(o.common.CampaignManifest); err != nil {
			return err
		}
	}
	var commandEnv []string
	if !o.common.DryRun {
		heads, err := eval.ResolveEvaluationHeads(ctx, o.common.CampaignManifest, o.smoke, setup)
		if err != nil {
			return err
		}
		commandEnv = heads.Environment()
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	adapterAction := action
	cleanupSmoke := func() {}
	if action == "smoke" {
		if o.replicas != 2 || o.measurementMinutes != 1 || o.warmupMinutes != 2 {
			return fmt.Errorf("E2 smoke requires two replicas, two-minute warmup, and one measurement minute")
		}
		o.e1Summary, o.reference, cleanupSmoke, err = makeSmokeInputs()
		if err != nil {
			return err
		}
		defer cleanupSmoke()
		adapterAction = "collect"
	}
	args := []string{adapterAction, "--profile", string(o.common.Profile), "--result-root", o.common.ResultRoot,
		"--minio-endpoint", clientEndpoint, "--repetitions", strconv.Itoa(o.repetitions)}
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
		if o.sloMultiplier != "5" || o.failureThreshold != "0.05" || o.ceilingMultiplier != "1" || o.warmupMinutes != 2 || o.steps != 20 || o.minutesPerStep != 1 || !o.noRetry {
			return fmt.Errorf("calibration is frozen at 5x, >5%%, ceiling 1, two-minute warmup, 20 one-minute steps, and --no-retry")
		}
		args = append(args, "--e1-summary", o.e1Summary, "--worker-cores", strconv.Itoa(o.workerCores),
			"--slo-multiplier", o.sloMultiplier, "--failure-threshold", o.failureThreshold,
			"--ceiling-multiplier", o.ceilingMultiplier, "--warmup-minutes", strconv.Itoa(o.warmupMinutes),
			"--steps", strconv.Itoa(o.steps), "--minutes-per-step", strconv.Itoa(o.minutesPerStep), "--no-retry")
	} else {
		if o.reference == "" || o.e1Summary == "" {
			return fmt.Errorf("collect requires --reference and --e1-summary")
		}
		if !o.smoke && o.replicas != 320 {
			return fmt.Errorf("claim collection requires --replicas 320")
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
	return (eval.Runner{DryRun: false}).Run(ctx, eval.Command{Name: filepath.Join(root, "run_rps_per_workload.sh"), Args: args, Dir: root, Env: commandEnv})
}

func makeSmokeInputs() (averages, reference string, cleanup func(), err error) {
	directory, err := os.MkdirTemp("", "nexus-e2-smoke-inputs-")
	if err != nil {
		return "", "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(directory) }
	averages = filepath.Join(directory, "non-claiming-averages.csv")
	reference = filepath.Join(directory, "non-claiming-reference.csv")
	averageRows := []string{"workload,unloaded_average_ms,n_samples"}
	referenceRows := []string{"workload,unloaded_average_ms,worker_cores,ceiling_multiplier,rbound,first_failing_step,first_failing_rps,rmax_b0,rref,status,reference_kind"}
	for _, workload := range []string{"helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving", "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer"} {
		averageRows = append(averageRows, workload+",1,1")
		referenceRows = append(referenceRows, workload+",1,1,1,1,,,1,1,BOUNDARY_OBSERVED,NON_CLAIMING_SMOKE_FIXTURE")
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

func fail(message string) { fmt.Fprintln(os.Stderr, "e2:", message); os.Exit(2) }
