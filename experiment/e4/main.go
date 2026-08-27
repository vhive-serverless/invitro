package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/vhive-serverless/loader/experiment/eval"
)

var workloads = []string{"helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving", "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer"}
var modes = []string{"invm-py", "nexus-py"}
var counts = []int{1, 2, 4, 6, 8, 10, 20, 30, 40}

type options struct {
	common                       eval.Config
	workloads, modes, countsText string
	warmup, sampleSeconds        int
	smoke                        bool
}

type cell struct {
	Workload string `json:"workload"`
	Mode     string `json:"mode"`
	Counts   []int  `json:"counts"`
}

type cellManifest struct {
	Cell           cell              `json:"cell"`
	Status         string            `json:"status"`
	Worker         string            `json:"worker"`
	CampaignSHA256 string            `json:"campaign_sha256,omitempty"`
	Started        string            `json:"started_utc"`
	Finished       string            `json:"finished_utc"`
	Artifacts      map[string]string `json:"artifacts,omitempty"`
	Error          string            `json:"error,omitempty"`
}

func main() {
	fs := flag.NewFlagSet("e4", flag.ContinueOnError)
	o := options{common: eval.Config{Profile: eval.Profile4}, workloads: strings.Join(workloads, ","),
		modes: strings.Join(modes, ","), countsText: "1,2,4,6,8,10,20,30,40", warmup: 1, sampleSeconds: 10}
	eval.AddFlags(fs, &o.common)
	fs.StringVar(&o.workloads, "workloads", o.workloads, "exact E4 workload list")
	fs.StringVar(&o.modes, "modes", o.modes, "exact E4 B0/N4 modes")
	fs.StringVar(&o.countsText, "instance-counts", o.countsText, "strictly increasing instance counts")
	fs.IntVar(&o.warmup, "warmup-successes-per-vm", 1, "fresh successes required per live VM and count")
	fs.IntVar(&o.sampleSeconds, "sample-seconds", 10, "steady invocation window before PSS sample")
	fs.BoolVar(&o.smoke, "smoke", false, "bounded non-claiming HelloWorld counts 1,2")
	if err := fs.Parse(os.Args[1:]); err != nil {
		fail(err)
	}
	if err := run(context.Background(), o); err != nil {
		fail(err)
	}
}

func run(ctx context.Context, o options) error {
	plan, setup, endpoint, err := makePlan(o)
	if err != nil {
		return err
	}
	for index, item := range plan {
		fmt.Printf("CELL ordinal=%d workload=%s mode=%s counts=%s\n", index, item.Workload, item.Mode, renderCounts(item.Counts))
	}
	fmt.Printf("PLAN experiment=e4 profile=%s cells=%d smoke=%t\n", o.common.Profile, len(plan), o.smoke)
	if o.common.DryRun {
		return nil
	}
	if err := eval.ValidateResultRoot(o.common.ResultRoot); err != nil {
		return err
	}
	if _, err := os.Stat(o.common.ResultRoot); err == nil {
		return fmt.Errorf("refusing to overwrite result root %s", o.common.ResultRoot)
	} else if !os.IsNotExist(err) {
		return err
	}
	campaignHash := ""
	if !o.smoke {
		if _, err := eval.RequireCampaign(o.common.CampaignManifest); err != nil {
			return err
		}
		campaignHash, err = eval.SHA256File(o.common.CampaignManifest)
		if err != nil {
			return err
		}
	}
	workerIPs := setup.LabeledIPs("loader-nodetype=worker")
	if len(workerIPs) != 1 {
		return fmt.Errorf("E4 requires exactly one worker")
	}
	worker, err := setup.URLForIP(workerIPs[0])
	if err != nil {
		return err
	}
	workerHome, err := eval.RemoteHome(worker)
	if err != nil {
		return err
	}
	var failures []error
	for _, item := range plan {
		cleanupOK, err := runCell(ctx, o, item, worker, workerHome, workerIPs[0], endpoint, campaignHash)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s/%s: %w", item.Workload, item.Mode, err))
			if !cleanupOK {
				return errors.Join(failures...)
			}
		}
	}
	return errors.Join(failures...)
}

func makePlan(o options) ([]cell, eval.Setup, string, error) {
	if o.common.Profile != eval.Profile4 {
		return nil, eval.Setup{}, "", fmt.Errorf("E4 requires profile 4-node")
	}
	if o.common.TopologyConfig == "" || o.common.ResultRoot == "" {
		return nil, eval.Setup{}, "", fmt.Errorf("--topology-config and --result-root are required")
	}
	setup, err := eval.LoadSetup(o.common.TopologyConfig)
	if err != nil {
		return nil, eval.Setup{}, "", err
	}
	if err := eval.ValidateSetup(setup, o.common.Profile); err != nil {
		return nil, eval.Setup{}, "", err
	}
	_, endpoint, err := eval.NormalizeMinioEndpoint(o.common.MinioEndpoint)
	if err != nil {
		return nil, eval.Setup{}, "", err
	}
	requestedWorkloads := split(o.workloads)
	requestedModes := split(o.modes)
	requestedCounts, err := parseCounts(o.countsText)
	if err != nil {
		return nil, eval.Setup{}, "", err
	}
	if o.warmup != 1 || o.sampleSeconds != 10 {
		return nil, eval.Setup{}, "", fmt.Errorf("E4 requires one fresh success and a ten-second sample window")
	}
	if o.smoke {
		if !equal(requestedWorkloads, []string{"helloworld"}) || !equal(requestedModes, modes) || !equalInts(requestedCounts, []int{1, 2}) {
			return nil, eval.Setup{}, "", fmt.Errorf("E4 smoke requires helloworld, both modes, and counts 1,2")
		}
	} else if !equal(requestedWorkloads, workloads) || !equal(requestedModes, modes) || !equalInts(requestedCounts, counts) {
		return nil, eval.Setup{}, "", fmt.Errorf("E4 claim run requires the frozen workloads, modes, and counts")
	}
	plan := make([]cell, 0, len(requestedWorkloads)*2)
	for index, workload := range requestedWorkloads {
		order := requestedModes
		if index%2 == 1 {
			order = []string{requestedModes[1], requestedModes[0]}
		}
		for _, mode := range order {
			plan = append(plan, cell{Workload: workload, Mode: mode, Counts: append([]int(nil), requestedCounts...)})
		}
	}
	return plan, setup, endpoint, nil
}

func runCell(ctx context.Context, o options, item cell, worker, workerHome, workerIP, endpoint, campaignHash string) (bool, error) {
	root := filepath.Join(o.common.ResultRoot, item.Workload, item.Mode)
	if err := eval.RemoteAbsent(ctx, worker, root); err != nil {
		return true, err
	}
	if err := os.MkdirAll(filepath.Dir(root), 0755); err != nil {
		return true, err
	}
	logPath := root + "-dispatch.log"
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return true, err
	}
	defer logFile.Close()
	started := time.Now().UTC().Format(time.RFC3339)
	fmt.Printf("ACQUISITION_START experiment=e4 workload=%s mode=%s worker=%s log=%s\n", item.Workload, item.Mode, worker, logPath)
	base := []string{"env", "--chdir=" + workerHome + "/khala", "NEXUS_MINIO_URL=http://" + endpoint}
	seedErr := runRemote(ctx, worker, logFile, append(base, "bash", "./scripts/deploy-minio-obj.sh", "http://"+endpoint)...)
	deployArgs := append(append([]string{}, base...), "./bin/khala-command",
		"--worker-config=internal/experiment/kn-integration-tracer/worker_node.json",
		"--vm-config=configs/vm_orchestrator_config.json", "--command=deploy", "--mode="+item.Mode,
		"--with-trace=true", "--debug=false", "--minio-endpoint="+endpoint)
	if item.Mode == "nexus-py" {
		deployArgs = append(deployArgs, "--vm-shmem-bytes=4194304", "--shmem-ring-bytes=4190208", "--shmem-io-quantum=262144")
	}
	deployErr := error(nil)
	if seedErr == nil {
		deployErr = runRemote(ctx, worker, logFile, deployArgs...)
	}
	snapshotErr := error(nil)
	if seedErr == nil && deployErr == nil {
		snapshotErr = runRemote(ctx, worker, logFile, append(base, "./bin/khala-command",
			"--worker-config=internal/experiment/kn-integration-tracer/worker_node.json",
			"--vm-config=configs/vm_orchestrator_config.json", "--command=create-snapshots", "--mode="+item.Mode,
			"--workload="+item.Workload, "--debug=false")...)
	}
	densityErr := error(nil)
	if seedErr == nil && deployErr == nil && snapshotErr == nil {
		densityErr = runRemote(ctx, worker, logFile, append(base, "sudo", "-n", "./bin/e4-density",
			"--workload="+item.Workload, "--mode="+item.Mode, "--worker-ip="+workerIP,
			"--instance-counts="+renderCounts(item.Counts), "--warmup-successes-per-vm=1", "--sample-seconds=10",
			"--result-root="+root)...)
	}
	cleanupErr := runRemote(ctx, worker, logFile, append(base, "./bin/khala-command",
		"--worker-config=internal/experiment/kn-integration-tracer/worker_node.json",
		"--vm-config=configs/vm_orchestrator_config.json", "--command=clean", "--mode="+item.Mode,
		"--remove-snapshots=true", "--debug=false")...)
	copyErr := eval.CopyRemoteTree(ctx, worker, root, logFile)
	cellErr := errors.Join(seedErr, deployErr, snapshotErr, densityErr, cleanupErr, copyErr)
	artifacts := map[string]string{}
	if copyErr == nil {
		for _, count := range item.Counts {
			manifestPath, manifestHash, verifyErr := verifyCount(root, count)
			if verifyErr != nil {
				cellErr = errors.Join(cellErr, verifyErr)
				continue
			}
			artifacts[filepath.Base(manifestPath)] = manifestHash
		}
	}
	status, message := "complete", ""
	if cellErr != nil {
		status, message = "failed", cellErr.Error()
	}
	manifest := cellManifest{Cell: item, Status: status, Worker: worker, CampaignSHA256: campaignHash,
		Started: started, Finished: time.Now().UTC().Format(time.RFC3339), Artifacts: artifacts, Error: message}
	if err := eval.CreateOnly(root+"-cell.json", manifest); err != nil {
		cellErr = errors.Join(cellErr, err)
	}
	if cellErr == nil {
		fmt.Printf("ACQUISITION_COMPLETE experiment=e4 workload=%s mode=%s result=%s\n", item.Workload, item.Mode, root)
	}
	return cleanupErr == nil, cellErr
}

func runRemote(ctx context.Context, worker string, output io.Writer, args ...string) error {
	fmt.Fprintln(output, "COMMAND", strings.Join(args, " "))
	command, err := eval.SSHCommand(ctx, worker, args...)
	if err != nil {
		return err
	}
	command.Stdout, command.Stderr = output, output
	if err := command.Run(); err != nil {
		return fmt.Errorf("remote command: %w", err)
	}
	return nil
}

func verifyCount(root string, count int) (string, string, error) {
	manifestPath := filepath.Join(root, "count-"+strconv.Itoa(count)+".manifest")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return manifestPath, "", err
	}
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			fields[parts[0]] = parts[1]
		}
	}
	if fields["status"] != "complete" || fields["cell"] != "count-"+strconv.Itoa(count) {
		return manifestPath, "", fmt.Errorf("non-terminal E4 count manifest %s", manifestPath)
	}
	csvPath := filepath.Join(root, "count-"+strconv.Itoa(count)+".csv")
	csvHash, err := eval.SHA256File(csvPath)
	if err != nil {
		return manifestPath, "", err
	}
	if fields["sha256"] != csvHash {
		return manifestPath, "", fmt.Errorf("E4 count checksum mismatch at %s", csvPath)
	}
	manifestHash, err := eval.SHA256File(manifestPath)
	return manifestPath, manifestHash, err
}

func parseCounts(text string) ([]int, error) {
	var result []int
	previous := 0
	for _, item := range strings.Split(text, ",") {
		value, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || value <= previous {
			return nil, fmt.Errorf("instance counts must be positive and strictly increasing")
		}
		result = append(result, value)
		previous = value
	}
	return result, nil
}

func split(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	result := strings.Split(text, ",")
	for i := range result {
		result[i] = strings.TrimSpace(result[i])
	}
	return result
}
func renderCounts(values []int) string {
	parts := make([]string, len(values))
	for index, value := range values {
		parts[index] = strconv.Itoa(value)
	}
	return strings.Join(parts, ",")
}
func equal(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
func fail(err error) { fmt.Fprintln(os.Stderr, "e4:", err); os.Exit(2) }
