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
	common                                                                                      eval.Config
	modes, reference, campaignLabel                                                             string
	startScale, step, endScale, shiftStep, divisor, warmupMinutes, repetitions, cooldownSeconds int
	pilotRun, allowExtendedEnd, smoke                                                           bool
}

func main() {
	fs := flag.NewFlagSet("e3", flag.ContinueOnError)
	o := options{common: eval.Config{Profile: eval.Profile4}, modes: "invm-py,nexus-py,nexus-rdma-py",
		startScale: 1, step: 1, endScale: 27, shiftStep: 10, divisor: 100,
		warmupMinutes: 2, repetitions: 1, cooldownSeconds: 120}
	eval.AddFlags(fs, &o.common)
	fs.StringVar(&o.modes, "modes", o.modes, "exact E3 mode set")
	fs.StringVar(&o.reference, "reference", "", "frozen E2 reference CSV")
	fs.IntVar(&o.startScale, "start-scale", 1, "initial trace scale")
	fs.IntVar(&o.step, "step", 1, "trace scale step")
	fs.IntVar(&o.endScale, "end-scale", 27, "final trace scale")
	fs.IntVar(&o.shiftStep, "shift-step", 10, "new functions per minute")
	fs.IntVar(&o.divisor, "divisor", 100, "reference RPS divisor")
	fs.IntVar(&o.warmupMinutes, "warmup-minutes", 2, "warmup minutes")
	fs.IntVar(&o.repetitions, "repetitions", 1, "campaign repetitions")
	fs.IntVar(&o.cooldownSeconds, "cooldown-seconds", 120, "cooldown between modes")
	fs.StringVar(&o.campaignLabel, "campaign-label", "", "topology interpretation label")
	fs.BoolVar(&o.pilotRun, "pilot-run", false, "mark a four-node non-claiming pilot")
	fs.BoolVar(&o.allowExtendedEnd, "allow-extended-end", false, "explicit later endpoint extension")
	fs.BoolVar(&o.smoke, "smoke", false, "bounded non-claiming smoke")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fail(err.Error())
	}
	if err := run(context.Background(), o); err != nil {
		fail(err.Error())
	}
}

func run(ctx context.Context, o options) error {
	if o.common.TopologyConfig == "" || o.common.ResultRoot == "" || o.reference == "" {
		return fmt.Errorf("--topology-config, --reference, and --result-root are required")
	}
	setup, err := eval.LoadSetup(o.common.TopologyConfig)
	if err != nil {
		return err
	}
	if err := eval.ValidateSetup(setup, o.common.Profile); err != nil {
		return err
	}
	_, endpoint, err := eval.NormalizeMinioEndpoint(o.common.MinioEndpoint)
	if err != nil {
		return err
	}
	if o.modes != "invm-py,nexus-py,nexus-rdma-py" {
		return fmt.Errorf("E3 requires exact B0/N4/N5 modes")
	}
	if o.repetitions != 1 || o.startScale != 1 || o.step != 1 || o.shiftStep != 10 || o.divisor != 100 || o.warmupMinutes != 2 {
		return fmt.Errorf("E3 single-pass trace contract is not frozen")
	}
	if o.endScale > 27 && !o.allowExtendedEnd {
		return fmt.Errorf("END_SCALE above 27 requires --allow-extended-end")
	}
	if o.smoke && (o.common.Profile != eval.Profile4 || o.endScale != 1 || o.cooldownSeconds != 0) {
		return fmt.Errorf("E3 smoke requires the 4-node profile, END_SCALE=1, and zero cooldown")
	}
	claimRun := o.common.Profile != eval.Profile4
	if o.common.Profile == eval.Profile4 {
		if !o.pilotRun || o.campaignLabel != "4-node-pilot" {
			return fmt.Errorf("4-node E3 requires --pilot-run --campaign-label 4-node-pilot")
		}
	} else if o.pilotRun {
		return fmt.Errorf("paper topology cannot be marked pilot")
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
	args := []string{"--profile", string(o.common.Profile), "--modes", o.modes, "--reference", o.reference,
		"--start-scale", strconv.Itoa(o.startScale), "--step", strconv.Itoa(o.step), "--end-scale", strconv.Itoa(o.endScale),
		"--shift-step", strconv.Itoa(o.shiftStep), "--divisor", strconv.Itoa(o.divisor), "--warmup-minutes", strconv.Itoa(o.warmupMinutes),
		"--repetitions", strconv.Itoa(o.repetitions), "--cooldown-seconds", strconv.Itoa(o.cooldownSeconds),
		"--minio-endpoint", endpoint, "--result-root", o.common.ResultRoot}
	if claimRun {
		args = append(args, "--claim-run")
	}
	if o.allowExtendedEnd {
		args = append(args, "--allow-extended-end")
	}
	if o.smoke {
		args = append(args, "--smoke")
	}
	if o.common.DryRun {
		args = append(args, "--dry-run")
	}
	return (eval.Runner{}).Run(ctx, eval.Command{Name: filepath.Join(root, "run_trace_ablation.sh"), Args: args, Dir: root, Env: commandEnv})
}

func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	return strings.TrimSpace(string(out)), err
}
func fail(message string) { fmt.Fprintln(os.Stderr, "e3:", message); os.Exit(2) }
