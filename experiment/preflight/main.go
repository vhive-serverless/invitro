package main

import (
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vhive-serverless/loader/experiment/eval"
	setupConfigs "github.com/vhive-serverless/loader/scripts/setup/configs"
)

type check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type artifact struct {
	Role   string `json:"role"`
	Host   string `json:"host"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type report struct {
	Profile             eval.Profile      `json:"profile"`
	MinioEndpoint       string            `json:"minio_endpoint"`
	Topology            eval.Setup        `json:"topology"`
	LiveNodes           []eval.LiveNode   `json:"live_nodes,omitempty"`
	TopologySHA256      string            `json:"topology_sha256"`
	Status              string            `json:"status"`
	AcquisitionStart    string            `json:"acquisition_start,omitempty"`
	QualificationRoot   string            `json:"qualification_root,omitempty"`
	QualificationSHA256 string            `json:"qualification_sha256,omitempty"`
	Checks              []check           `json:"checks"`
	Provenance          []eval.Provenance `json:"provenance,omitempty"`
	Artifacts           []artifact        `json:"artifacts,omitempty"`
}

type vmConfig struct {
	RootfsPath      string
	KernelPath      string
	FirecrackerPath string
	JailerPath      string
}

type kubePods struct {
	Items []struct {
		Metadata struct{ Namespace, Name string } `json:"metadata"`
		Status   struct {
			Phase             string                 `json:"phase"`
			ContainerStatuses []struct{ Ready bool } `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

var sshTargetPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9.-]+$`)

const e4SnapshotCleanupPolicy = "initial-purge;normal-preserve;setup-recovery-purge;campaign-final-purge"

func main() {
	args := os.Args[1:]
	freezeSubcommand := false
	if len(args) > 0 && args[0] == "freeze" {
		freezeSubcommand = true
		args = args[1:]
	}
	fs := flag.NewFlagSet("preflight", flag.ContinueOnError)
	cfg := eval.Config{Profile: eval.Profile4}
	eval.AddFlags(fs, &cfg)
	smokeRoot := fs.String("smoke-root", "", "verified E1 smoke result root required for freeze")
	if err := fs.Parse(args); err != nil {
		fail(err.Error())
	}
	if freezeSubcommand {
		cfg.Freeze = true
	}
	code, err := run(context.Background(), cfg, *smokeRoot)
	if err != nil {
		fmt.Fprintln(os.Stderr, "preflight:", err)
	}
	os.Exit(code)
}

func run(ctx context.Context, cfg eval.Config, smokeRoot string) (int, error) {
	if cfg.Freeze && cfg.ResultRoot == "" && cfg.CampaignManifest != "" {
		cfg.ResultRoot = filepath.Dir(cfg.CampaignManifest)
	}
	if cfg.TopologyConfig == "" || cfg.ResultRoot == "" || cfg.MinioEndpoint == "" {
		return 2, fmt.Errorf("--topology-config, --minio-endpoint, and --result-root are required")
	}
	setup, err := eval.LoadSetup(cfg.TopologyConfig)
	if err != nil {
		return 2, err
	}
	if err := eval.ValidateSetup(setup, cfg.Profile); err != nil {
		return 2, err
	}
	baseURL, _, err := eval.NormalizeMinioEndpoint(cfg.MinioEndpoint)
	if err != nil {
		return 2, err
	}
	rep := report{Profile: cfg.Profile, MinioEndpoint: baseURL, Topology: setup, Status: "CHECKING"}
	rep.TopologySHA256, _ = eval.SHA256File(cfg.TopologyConfig)
	if cfg.DryRun {
		for _, name := range plannedChecks(cfg.Freeze) {
			rep.Checks = append(rep.Checks, check{Name: name, Status: "PLANNED"})
		}
		rep.Status = "DRY_RUN_READY"
		data, _ := json.MarshalIndent(rep, "", "  ")
		fmt.Println(string(data))
		return 0, nil
	}

	checker := &checks{ctx: ctx, report: &rep}
	checker.localGit("invitro", ".", eval.InVitroBranch)
	checker.localGit("khala", "../khala", eval.KhalaBranch)
	checker.localGit("firecracker", "../firecracker", eval.FirecrackerBranch)
	checker.remoteSource("firecracker", eval.FirecrackerOrigin, eval.FirecrackerBranch, eval.FirecrackerHead)
	rdmaHead := checker.remoteBranch("rdma", eval.RDMAOrigin, eval.RDMABranch)
	nodes, nodeErr := eval.KubectlNodes()
	checker.record("kubernetes_nodes", nodeErr, "")
	if nodeErr == nil {
		topologyErr := eval.ValidateLive(nodes, setup, cfg.Profile)
		checker.record("kubernetes_topology", topologyErr, "")
		if topologyErr == nil {
			rep.LiveNodes = nodes
		}
	}
	checker.httpHealth("minio_loader", eval.MinioHealthURL(baseURL))
	checker.kubernetesWorkloads()
	prometheusOutput, prometheusErr := checker.capture("scripts/util/wait_prometheus_ready.sh")
	checker.record("prometheus_api_ready", prometheusErr, prometheusOutput)

	localInvitro, _ := eval.GitProvenance(".")
	localKhala, _ := eval.GitProvenance("../khala")
	for _, ip := range setup.LabeledIPs("loader-nodetype=worker") {
		target, mapErr := setup.URLForIP(ip)
		checker.record("worker_url_"+ip, mapErr, target)
		if mapErr != nil {
			continue
		}
		checker.remoteKVM(target)
		checker.remoteWorkerTools(target)
		checker.remoteMinio(target, baseURL)
		checker.remoteGit("worker_khala", target, remoteHome(target)+"/khala", localKhala.Head, eval.KhalaBranch)
		checker.remoteGit("worker_firecracker", target, remoteHome(target)+"/firecracker", eval.FirecrackerHead, eval.FirecrackerBranch)
		checker.remoteRuntimeSnapshots(target)
		checker.workerArtifacts(target)
	}
	loaderIPs := setup.LabeledIPs("loader-nodetype=monitoring")
	if len(loaderIPs) == 1 {
		loaderTarget, mapErr := setup.URLForIP(loaderIPs[0])
		checker.record("loader_url", mapErr, loaderTarget)
		if mapErr == nil {
			checker.remoteGit("loader_invitro", loaderTarget, remoteHome(loaderTarget)+"/loader", localInvitro.Head, eval.InVitroBranch)
		}
	}
	for _, ip := range setup.LabeledIPs("minio-type=tenant") {
		target, mapErr := setup.URLForIP(ip)
		checker.record("tenant_url_"+ip, mapErr, target)
		if mapErr != nil {
			continue
		}
		checker.remoteGit("tenant_rdma", target, remoteHome(target)+"/rdma-demo", rdmaHead, eval.RDMABranch)
		checker.remoteRDMA(target)
	}
	if cfg.Freeze {
		checker.smokeEvidence(smokeRoot)
	}

	failed := checker.failed()
	switch {
	case failed:
		rep.Status = "BLOCKED"
	case cfg.Freeze:
		rep.Status = "ACQUISITION_START"
		rep.AcquisitionStart = time.Now().UTC().Format(time.RFC3339)
	default:
		rep.Status = "READY_FOR_SMOKE"
	}
	out := filepath.Join(cfg.ResultRoot, "preflight.json")
	if cfg.Freeze {
		if cfg.CampaignManifest == "" {
			checker.record("campaign_manifest", fmt.Errorf("--campaign-manifest is required for freeze"), "")
			rep.Status = "BLOCKED"
			failed = true
		} else {
			out = cfg.CampaignManifest
		}
	}
	if err := eval.CreateOnly(out, rep); err != nil {
		return 2, err
	}
	if failed {
		fmt.Printf("PREFLIGHT_BLOCKED result=%s\n", out)
		return 2, fmt.Errorf("one or more archived preflight checks failed")
	}
	fmt.Printf("PREFLIGHT_READY status=%s result=%s\n", rep.Status, out)
	return 0, nil
}

type checks struct {
	ctx    context.Context
	report *report
}

func (c *checks) record(name string, err error, detail string) {
	status := "PASS"
	if err != nil {
		status = "FAIL"
		if detail != "" {
			detail += ": "
		}
		detail += err.Error()
	}
	c.report.Checks = append(c.report.Checks, check{Name: name, Status: status, Detail: detail})
}

func (c *checks) failed() bool {
	for _, item := range c.report.Checks {
		if item.Status == "FAIL" {
			return true
		}
	}
	return false
}

func (c *checks) localGit(label, path, branch string) {
	p, err := eval.GitProvenance(path)
	if err == nil {
		err = p.ValidateClean()
		if err == nil && p.Branch != branch {
			err = fmt.Errorf("branch %s, want %s", p.Branch, branch)
		}
		c.report.Provenance = append(c.report.Provenance, p)
	}
	c.record("local_git_"+label, err, path)
}

func (c *checks) remoteSource(label, repository, branch, expectedHead string) {
	command := exec.CommandContext(c.ctx, "git", "ls-remote", "--heads", repository, "refs/heads/"+branch)
	command.Env = append(os.Environ(), "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=10")
	output, err := command.CombinedOutput()
	head := ""
	if err == nil {
		head, err = parseRemoteHead(string(output), branch)
	}
	if err == nil && head != expectedHead {
		err = fmt.Errorf("remote HEAD %q, want %s", head, expectedHead)
	}
	if err != nil && len(output) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	if err == nil {
		c.report.Provenance = append(c.report.Provenance, eval.Provenance{Repository: repository, Head: expectedHead, Branch: branch})
	}
	c.record("source_"+label, err, repository+" "+branch)
}

func (c *checks) remoteBranch(label, repository, branch string) string {
	command := exec.CommandContext(c.ctx, "git", "ls-remote", "--heads", repository, "refs/heads/"+branch)
	output, err := command.CombinedOutput()
	head := ""
	if err == nil {
		head, err = parseRemoteHead(string(output), branch)
		if err == nil {
			c.report.Provenance = append(c.report.Provenance, eval.Provenance{Repository: repository, Head: head, Branch: branch})
		}
	}
	if err != nil && len(output) > 0 {
		err = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	c.record("source_"+label, err, repository+" "+branch)
	return head
}

func parseRemoteHead(output, branch string) (string, error) {
	wantRef := "refs/heads/" + branch
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && len(fields[0]) == 40 && fields[1] == wantRef {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("malformed remote HEAD %q", strings.TrimSpace(output))
}

func (c *checks) httpHealth(name, endpoint string) {
	request, err := http.NewRequestWithContext(c.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		c.record(name, err, endpoint)
		return
	}
	client := http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err == nil {
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			err = fmt.Errorf("HTTP %d", response.StatusCode)
		}
	}
	c.record(name, err, endpoint)
}

func (c *checks) capture(name string, args ...string) (string, error) {
	out, err := exec.CommandContext(c.ctx, name, args...).CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func (c *checks) ssh(target string, command ...string) (string, error) {
	if !sshTargetPattern.MatchString(target) {
		return "", fmt.Errorf("invalid SSH target %q", target)
	}
	args := []string{"-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10", target}
	args = append(args, command...)
	return c.capture("ssh", args...)
}

func remoteHome(target string) string { return "/users/" + strings.SplitN(target, "@", 2)[0] }

const runtimeSnapshotsCheckScript = `if [ ! -d "$1" ]; then exit 0; fi
find "$1" -mindepth 1 ! -path "$1/.gitkeep" -print -quit`

func runtimeSnapshotsCommand(path string) []string {
	return []string{"sh", "-c", shellQuote(runtimeSnapshotsCheckScript), "preflight-runtime-snapshots", shellQuote(path)}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func parseRuntimeSnapshotsOutput(output, path string) error {
	path = strings.TrimRight(path, "/")
	allowed := path + "/.gitkeep"
	var stale []string
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "Warning: Permanently added ") {
			continue
		}
		if line == allowed {
			continue
		}
		if !strings.HasPrefix(line, path+"/") {
			return fmt.Errorf("unexpected runtime snapshots output %q", line)
		}
		stale = append(stale, line)
	}
	if len(stale) > 0 {
		return fmt.Errorf("runtime snapshots directory contains stale entries: %s", strings.Join(stale, ", "))
	}
	return nil
}

func (c *checks) remoteRuntimeSnapshots(target string) {
	path := remoteHome(target) + "/khala/runtime/snapshots"
	output, err := c.ssh(target, runtimeSnapshotsCommand(path)...)
	if err == nil {
		err = parseRuntimeSnapshotsOutput(output, path)
	}
	c.record("worker_runtime_snapshots_"+target, err, path)
}

func (c *checks) remoteKVM(target string) {
	_, err := c.ssh(target, "test", "-r", "/dev/kvm")
	if err == nil {
		_, err = c.ssh(target, "test", "-w", "/dev/kvm")
	}
	c.record("worker_kvm_"+target, err, "/dev/kvm")
}

func (c *checks) remoteWorkerTools(target string) {
	home := remoteHome(target)
	mcPath := home + "/minio-binaries/mc"
	mcVersion, mcErr := c.ssh(target, mcPath, "--version")
	c.record("worker_mc_"+target, mcErr, mcVersion)

	goVersion, goErr := c.ssh(target, "/usr/local/go/bin/go", "version")
	if goErr == nil {
		_, goErr = c.ssh(target, "grep", "-Fq", "/usr/local/go/bin", "/etc/profile")
	}
	c.record("worker_go_profile_"+target, goErr, goVersion)

	setup, setupErr := setupConfigs.GetSetupJSON("scripts/setup/configs")
	c.record("worker_flamegraph_config_"+target, setupErr, "scripts/setup/configs/setup.json")
	if setupErr != nil {
		return
	}
	flameGraphPath := home + "/FlameGraph"
	head, flameGraphErr := c.ssh(target, "git", "-C", flameGraphPath, "rev-parse", "HEAD")
	if flameGraphErr == nil && head != setup.FlameGraphCommit {
		flameGraphErr = fmt.Errorf("HEAD %s, want %s", head, setup.FlameGraphCommit)
	}
	origin, originErr := c.ssh(target, "git", "-C", flameGraphPath, "config", "--get", "remote.origin.url")
	if flameGraphErr == nil {
		flameGraphErr = originErr
	}
	if flameGraphErr == nil && origin != setup.FlameGraphRepo {
		flameGraphErr = fmt.Errorf("origin %s, want %s", origin, setup.FlameGraphRepo)
	}
	status, statusErr := c.ssh(target, "git", "-C", flameGraphPath, "status", "--porcelain")
	if flameGraphErr == nil {
		flameGraphErr = statusErr
	}
	if flameGraphErr == nil && status != "" {
		flameGraphErr = fmt.Errorf("tree dirty: %s", status)
	}
	for _, script := range []string{"stackcollapse-perf.pl", "flamegraph.pl"} {
		full := filepath.Join(flameGraphPath, script)
		_, scriptErr := c.ssh(target, "test", "-x", full)
		if flameGraphErr == nil {
			flameGraphErr = scriptErr
		}
		if scriptErr == nil {
			output, hashErr := c.ssh(target, "sha256sum", full)
			fields := strings.Fields(output)
			if hashErr == nil && len(fields) == 2 {
				c.report.Artifacts = append(c.report.Artifacts, artifact{Role: "worker", Host: target, Path: "FlameGraph/" + script, SHA256: fields[0]})
			}
			if flameGraphErr == nil {
				flameGraphErr = hashErr
			}
		}
	}
	c.record("worker_flamegraph_"+target, flameGraphErr, flameGraphPath)
}

func (c *checks) remoteMinio(target, baseURL string) {
	_, err := c.ssh(target, "curl", "-fsS", "--max-time", "5", eval.MinioHealthURL(baseURL))
	c.record("worker_minio_"+target, err, baseURL)
}

func (c *checks) remoteGit(role, target, path, wantHead, wantBranch string) {
	head, err := c.ssh(target, "git", "-C", path, "rev-parse", "HEAD")
	if err == nil && wantHead != "" && head != wantHead {
		err = fmt.Errorf("HEAD %s, want %s", head, wantHead)
	}
	branch, branchErr := c.ssh(target, "git", "-C", path, "branch", "--show-current")
	if err == nil {
		err = branchErr
	}
	if err == nil && branch != wantBranch {
		err = fmt.Errorf("branch %s, want %s", branch, wantBranch)
	}
	status, statusErr := c.ssh(target, "git", "-C", path, "status", "--porcelain")
	if err == nil {
		err = statusErr
	}
	if err == nil && status != "" {
		err = fmt.Errorf("tree dirty: %s", status)
	}
	if err == nil {
		c.report.Provenance = append(c.report.Provenance, eval.Provenance{Repository: target + ":" + path, Head: head, Branch: branch, Status: status})
	}
	c.record(role+"_"+target, err, path)
}

func (c *checks) workerArtifacts(target string) {
	configPaths := []string{"configs/vm_orchestrator_config.json", "configs/vm_orchestrator_config_js.json"}
	configs := make([]vmConfig, 0, len(configPaths))
	for _, relative := range configPaths {
		data, err := os.ReadFile(filepath.Join("../khala", relative))
		if err == nil {
			var cfg vmConfig
			err = json.Unmarshal(data, &cfg)
			if err == nil {
				configs = append(configs, cfg)
			}
		}
		c.record("local_config_"+filepath.Base(relative), err, relative)
	}
	if len(configs) != 2 {
		return
	}
	if configs[0].RootfsPath != configs[1].RootfsPath {
		c.record("unified_rootfs", fmt.Errorf("config rootfs paths differ"), "")
		return
	}
	c.record("unified_rootfs", nil, configs[0].RootfsPath)
	paths := []string{configPaths[0], configPaths[1], configs[0].RootfsPath, configs[0].KernelPath,
		configs[0].FirecrackerPath, configs[0].JailerPath, "bin/kn-integration", "bin/nexus-backend",
		"bin/hardware-manager", "bin/vm-orchestrator", "bin/khala-command", "bin/kn-integration-tracer",
		"bin/e4-density"}
	for _, relative := range paths {
		localPath := filepath.Join("../khala", relative)
		full := filepath.Join(remoteHome(target), "khala", relative)
		localOutput, err := c.capture("sha256sum", localPath)
		output := ""
		if err == nil {
			output, err = c.ssh(target, "sha256sum", full)
		}
		digest := ""
		if err == nil {
			localDigest, remoteDigest, matchErr := matchingSHA256(localOutput, output)
			err = matchErr
			if err == nil {
				digest = remoteDigest
				c.report.Artifacts = append(c.report.Artifacts, artifact{Role: "loader-reference", Host: "local", Path: relative, SHA256: localDigest})
				if relative == configs[0].FirecrackerPath && digest != eval.FirecrackerSHA256 {
					err = fmt.Errorf("Firecracker SHA-256 %s, want %s", digest, eval.FirecrackerSHA256)
				}
				if relative == configs[0].KernelPath && digest != eval.KernelSHA256 {
					err = fmt.Errorf("kernel SHA-256 %s, want %s", digest, eval.KernelSHA256)
				}
				c.report.Artifacts = append(c.report.Artifacts, artifact{Role: "worker", Host: target, Path: relative, SHA256: digest})
			}
		}
		c.record("artifact_"+sanitize(target+"_"+relative), err, relative)
	}
}

func matchingSHA256(localOutput, remoteOutput string) (string, string, error) {
	parse := func(label, output string) (string, error) {
		fields := strings.Fields(output)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return "", fmt.Errorf("malformed %s sha256sum output", label)
		}
		return fields[0], nil
	}
	localDigest, err := parse("local", localOutput)
	if err != nil {
		return "", "", err
	}
	remoteDigest, err := parse("remote", remoteOutput)
	if err != nil {
		return "", "", err
	}
	if localDigest != remoteDigest {
		return localDigest, remoteDigest, fmt.Errorf("worker SHA-256 %s, loader reference %s", remoteDigest, localDigest)
	}
	return localDigest, remoteDigest, nil
}

func (c *checks) remoteRDMA(target string) {
	home := remoteHome(target)
	binary := home + "/rdma-demo/s3-rdma-server"
	_, err := c.ssh(target, "test", "-x", binary)
	c.record("rdma_server_binary_"+target, err, binary)
	if err == nil {
		output, hashErr := c.ssh(target, "sha256sum", binary)
		fields := strings.Fields(output)
		if hashErr == nil && len(fields) == 2 {
			c.report.Artifacts = append(c.report.Artifacts, artifact{Role: "tenant", Host: target, Path: "s3-rdma-server", SHA256: fields[0]})
		}
		c.record("rdma_server_hash_"+target, hashErr, "s3-rdma-server")
	}
	output, err := c.ssh(target, "ibv_devices")
	if err == nil && len(strings.Fields(output)) < 3 {
		err = fmt.Errorf("no RDMA device listed")
	}
	c.record("rdma_device_"+target, err, output)
}

func (c *checks) kubernetesWorkloads() {
	output, err := c.capture("kubectl", "get", "pods", "-A", "-o", "json")
	if err != nil {
		c.record("kubernetes_workloads", err, "")
		return
	}
	var pods kubePods
	if err = json.Unmarshal([]byte(output), &pods); err != nil {
		c.record("kubernetes_workloads", err, "")
		return
	}
	required := map[string]bool{"istio-system": false, "knative-serving": false, "minio": false}
	prometheus := false
	var failures []string
	for _, pod := range pods.Items {
		if _, ok := required[pod.Metadata.Namespace]; ok {
			required[pod.Metadata.Namespace] = true
		}
		lower := strings.ToLower(pod.Metadata.Namespace + "/" + pod.Metadata.Name)
		if strings.Contains(lower, "prometheus") {
			prometheus = true
		}
		if pod.Status.Phase == "Succeeded" {
			continue
		}
		if pod.Status.Phase != "Running" {
			failures = append(failures, pod.Metadata.Namespace+"/"+pod.Metadata.Name+":"+pod.Status.Phase)
			continue
		}
		for _, status := range pod.Status.ContainerStatuses {
			if !status.Ready {
				failures = append(failures, pod.Metadata.Namespace+"/"+pod.Metadata.Name+":not-ready")
			}
		}
	}
	for namespace, found := range required {
		if !found {
			failures = append(failures, "missing namespace workload "+namespace)
		}
	}
	if !prometheus {
		failures = append(failures, "missing Prometheus pod")
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		err = errors.New(strings.Join(failures, "; "))
	}
	c.record("kubernetes_workloads", err, fmt.Sprintf("pods=%d", len(pods.Items)))
}

func (c *checks) smokeEvidence(root string) {
	if root == "" {
		c.record("guest_minio_smoke", fmt.Errorf("--smoke-root is required for freeze"), "")
		return
	}
	var manifests, e4Cells, e4InitialCleanups []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && entry.Name() == "manifest.txt" {
			manifests = append(manifests, path)
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), "-cell.json") {
			e4Cells = append(e4Cells, path)
		}
		if !entry.IsDir() && entry.Name() == "initial-cleanup.json" {
			e4InitialCleanups = append(e4InitialCleanups, path)
		}
		return nil
	})
	e1, e2, e3 := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, path := range manifests {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			err = errors.Join(err, readErr)
			continue
		}
		text := string(data)
		if !strings.Contains(text, "smoke=true\n") || !strings.Contains(text, "exit_status=0\n") {
			continue
		}
		fields := lineFields(text)
		switch {
		case strings.HasPrefix(fields["claim_id"], "e1-smoke-"):
			if lifecycleErr := validateE1LifecycleSmokeManifest(fields); lifecycleErr != nil {
				err = errors.Join(err, fmt.Errorf("E1 %s: %w", fields["claim_id"], lifecycleErr))
			} else {
				e1[fields["claim_id"]] = true
			}
		case fields["phase"] == "collection" && fields["workload"] == "helloworld":
			if lifecycleErr := validateLifecycleSmokeManifest(fields, "E2"); lifecycleErr != nil {
				err = errors.Join(err, fmt.Errorf("E2 %s: %w", fields["mode"], lifecycleErr))
			} else if checksumErr := validateArchivedOutputChecksums(filepath.Dir(path)); checksumErr != nil {
				err = errors.Join(err, fmt.Errorf("E2 %s: %w", fields["mode"], checksumErr))
			} else {
				e2[fields["mode"]] = true
			}
		case fields["experiment"] == "e3" && fields["end_scale"] == "1" && fields["claim_bearing"] == "false":
			if lifecycleErr := validateLifecycleSmokeManifest(fields, "E3"); lifecycleErr != nil {
				err = errors.Join(err, fmt.Errorf("E3 %s: %w", fields["mode"], lifecycleErr))
			} else if checksumErr := validateArchivedOutputChecksums(filepath.Dir(path)); checksumErr != nil {
				err = errors.Join(err, fmt.Errorf("E3 %s: %w", fields["mode"], checksumErr))
			} else {
				e3[fields["mode"]] = true
			}
		}
	}
	e4 := map[string]bool{}
	for _, path := range e4Cells {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			err = errors.Join(err, readErr)
			continue
		}
		var manifest struct {
			ManifestVersion    int    `json:"manifest_version"`
			Status             string `json:"status"`
			Phase              string `json:"phase"`
			SetupAttempts      int    `json:"setup_attempts"`
			AcquisitionStarted bool   `json:"acquisition_started"`
			CleanupSucceeded   bool   `json:"cleanup_succeeded"`
			VerificationDone   bool   `json:"verification_completed"`
			SnapshotPolicy     string `json:"snapshot_cleanup_policy"`
			Cell               struct {
				Workloads            []string `json:"workloads"`
				Mode                 string   `json:"mode"`
				InstancesPerWorkload []int    `json:"instances_per_workload"`
			} `json:"cell"`
		}
		if json.Unmarshal(data, &manifest) == nil && manifest.ManifestVersion == 3 && manifest.Status == "complete" && manifest.Phase == "verify" &&
			manifest.SetupAttempts >= 1 && manifest.SetupAttempts <= 2 && manifest.AcquisitionStarted &&
			manifest.CleanupSucceeded && manifest.VerificationDone &&
			manifest.SnapshotPolicy == e4SnapshotCleanupPolicy &&
			len(manifest.Cell.Workloads) == 1 && manifest.Cell.Workloads[0] == "helloworld" &&
			manifest.Cell.Mode != "" && len(manifest.Cell.InstancesPerWorkload) == 2 &&
			manifest.Cell.InstancesPerWorkload[0] == 1 && manifest.Cell.InstancesPerWorkload[1] == 2 {
			e4[manifest.Cell.Mode] = true
		}
	}
	e4InitialCleanup := false
	if len(e4InitialCleanups) != 1 {
		err = errors.Join(err, fmt.Errorf("expected one E4 initial cleanup record, got %d", len(e4InitialCleanups)))
	} else {
		path := e4InitialCleanups[0]
		data, readErr := os.ReadFile(path)
		var manifest struct {
			ManifestVersion int    `json:"manifest_version"`
			Status          string `json:"status"`
			LogSHA256       string `json:"log_sha256"`
			RemoveSnapshots bool   `json:"remove_snapshots"`
			SnapshotPolicy  string `json:"snapshot_cleanup_policy"`
		}
		if readErr != nil || json.Unmarshal(data, &manifest) != nil || manifest.ManifestVersion != 1 ||
			manifest.Status != "complete" || !manifest.RemoveSnapshots || manifest.SnapshotPolicy != e4SnapshotCleanupPolicy {
			err = errors.Join(err, fmt.Errorf("invalid E4 initial cleanup record %s", path))
		} else {
			logHash, hashErr := eval.SHA256File(filepath.Join(filepath.Dir(path), "initial-cleanup.log"))
			if hashErr != nil || manifest.LogSHA256 != logHash {
				err = errors.Join(err, fmt.Errorf("invalid E4 initial cleanup log binding %s", path))
			} else {
				e4InitialCleanup = true
			}
		}
	}
	wants := []struct {
		name string
		got  map[string]bool
		keys []string
	}{
		{"E1", e1, []string{"e1-smoke-2b", "e1-smoke-4mib"}},
		{"E2", e2, []string{"invm-py", "nexus-py"}},
		{"E3", e3, []string{"invm-py", "nexus-py", "nexus-rdma-py"}},
		{"E4", e4, []string{"invm-py", "nexus-py"}},
	}
	for _, want := range wants {
		for _, key := range want.keys {
			if !want.got[key] {
				err = errors.Join(err, fmt.Errorf("missing terminal %s smoke %s", want.name, key))
			}
		}
	}
	if !e4InitialCleanup {
		err = errors.Join(err, fmt.Errorf("missing terminal E4 initial cleanup evidence"))
	}
	c.report.QualificationRoot = root
	treeDigest, digestErr := directorySHA256(root)
	if digestErr != nil {
		err = errors.Join(err, fmt.Errorf("qualification tree digest: %w", digestErr))
	} else {
		c.report.QualificationSHA256 = treeDigest
	}
	c.record("e1_e4_smoke_evidence", err, root)
}

func validateE1LifecycleSmokeManifest(fields map[string]string) error {
	if fields["manifest_version"] != "9" {
		return fmt.Errorf("manifest_version=%q, want 9", fields["manifest_version"])
	}
	if fields["cell_status_sequence"] != "started,complete" || fields["acquisition_retry"] != "false" ||
		fields["independent_continuation"] != "true" || fields["contamination_stop"] != "true" {
		return fmt.Errorf("incomplete E1 lifecycle contract")
	}
	if fields["fixture_setup_max_attempts"] != "2" || fields["cell_setup_max_attempts"] != "2" {
		return fmt.Errorf("unbounded E1 setup contract")
	}
	attempts, err := strconv.Atoi(fields["fixture_setup_attempts"])
	if err != nil || attempts < 1 || attempts > 2 {
		return fmt.Errorf("fixture_setup_attempts=%q, want an integer in [1,2]", fields["fixture_setup_attempts"])
	}
	return nil
}

// validateLifecycleSmokeManifest admits only a terminal cell manifest.  The
// setup/deploy retry is bounded to two attempts, while loader/acquisition is
// single-shot; a successful cleanup and explicit final marker prove that the
// archive is not a pre-loader or partially finalized cell.
func validateLifecycleSmokeManifest(fields map[string]string, experiment string) error {
	if fields["manifest_version"] != "2" {
		return fmt.Errorf("manifest_version=%q, want 2", fields["manifest_version"])
	}
	if fields["lifecycle_phase"] != "final" {
		return fmt.Errorf("lifecycle_phase=%q, want final", fields["lifecycle_phase"])
	}
	if fields["loader_started"] != "true" {
		return fmt.Errorf("loader_started=%q, want true", fields["loader_started"])
	}
	if fields["cleanup_exit_status"] != "0" {
		return fmt.Errorf("cleanup_exit_status=%q, want 0", fields["cleanup_exit_status"])
	}
	if fields["exit_status"] != "0" {
		return fmt.Errorf("exit_status=%q, want 0", fields["exit_status"])
	}
	if experiment == "E2" && fields["evidence_status"] != "0" {
		return fmt.Errorf("evidence_status=%q, want 0", fields["evidence_status"])
	}
	for _, key := range []string{"setup_attempts", "deploy_attempts", "deploy_invocations"} {
		value, err := strconv.Atoi(fields[key])
		if err != nil || value < 1 || value > 2 {
			return fmt.Errorf("%s=%q, want an integer in [1,2]", key, fields[key])
		}
	}
	setupAttempts, _ := strconv.Atoi(fields["setup_attempts"])
	deployAttempts, _ := strconv.Atoi(fields["deploy_attempts"])
	deployInvocations, _ := strconv.Atoi(fields["deploy_invocations"])
	if deployAttempts != deployInvocations || deployInvocations > setupAttempts {
		return fmt.Errorf("deploy attempts/invocations inconsistent: setup=%d deploy=%d invocations=%d", setupAttempts, deployAttempts, deployInvocations)
	}
	return nil
}

func validateArchivedOutputChecksums(directory string) error {
	path := filepath.Join(directory, "archived-output-checksums.csv")
	handle, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer handle.Close()
	reader := csv.NewReader(handle)
	header, err := reader.Read()
	if err != nil || len(header) != 2 || header[0] != "path" || header[1] != "sha256" {
		return fmt.Errorf("%s has invalid header", path)
	}
	seen := map[string]bool{}
	count := 0
	for {
		row, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil || len(row) != 2 {
			return fmt.Errorf("%s has a malformed row", path)
		}
		relative := filepath.Clean(row[0])
		if relative == "." || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s has unsafe path %q", path, row[0])
		}
		if seen[relative] {
			return fmt.Errorf("%s repeats path %q", path, relative)
		}
		seen[relative] = true
		if len(row[1]) != sha256.Size*2 {
			return fmt.Errorf("%s has invalid digest for %q", path, relative)
		}
		actual, hashErr := eval.SHA256File(filepath.Join(directory, relative))
		if hashErr != nil {
			return fmt.Errorf("%s cannot hash %q: %w", path, relative, hashErr)
		}
		if actual != row[1] {
			return fmt.Errorf("%s digest mismatch for %q", path, relative)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("%s contains no artifact rows", path)
	}
	return nil
}

func directorySHA256(root string) (string, error) {
	var records []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("qualification tree contains symlink %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		digest, err := eval.SHA256File(path)
		if err != nil {
			return err
		}
		records = append(records, digest+"  "+filepath.ToSlash(relative)+"\n")
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", fmt.Errorf("qualification tree %s has no files", root)
	}
	sort.Strings(records)
	digest := sha256.New()
	for _, record := range records {
		_, _ = io.WriteString(digest, record)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func lineFields(text string) map[string]string {
	result := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	return result
}

func sanitize(value string) string {
	return strings.NewReplacer("/", "_", ":", "_", "@", "_").Replace(value)
}

func plannedChecks(freeze bool) []string {
	values := []string{"local_git", "kubernetes_nodes", "kubernetes_topology", "minio_loader", "kubernetes_workloads", "prometheus_api_ready", "worker_kvm", "worker_tools", "worker_flamegraph", "worker_minio", "worker_runtime_snapshots", "deployed_git", "unified_rootfs", "artifact_hashes", "rdma"}
	if freeze {
		values = append(values, "e1_e4_smoke_evidence")
	}
	return values
}

func fail(message string) { fmt.Fprintln(os.Stderr, "preflight:", message); os.Exit(2) }
