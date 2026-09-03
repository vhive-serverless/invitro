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
	ActivatorUID        string            `json:"activator_uid,omitempty"`
	ActivatorGeneration int64             `json:"activator_generation,omitempty"`
	QualificationRoot   string            `json:"qualification_root,omitempty"`
	QualificationSHA256 string            `json:"qualification_sha256,omitempty"`
	QualificationScope  string            `json:"qualification_scope,omitempty"`
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

var captureActivatorIdentity = eval.CaptureActivatorIdentity

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
	scope := fs.String("scope", "all", "qualification scope for freeze: all or e1")
	if err := fs.Parse(args); err != nil {
		fail(err.Error())
	}
	if freezeSubcommand {
		cfg.Freeze = true
	}
	if *scope != "all" && *scope != "e1" {
		fail("--scope must be all or e1")
	}
	if !cfg.Freeze && *scope != "all" {
		fail("--scope is accepted only for freeze")
	}
	code, err := runScoped(context.Background(), cfg, *smokeRoot, *scope)
	if err != nil {
		fmt.Fprintln(os.Stderr, "preflight:", err)
	}
	os.Exit(code)
}

func run(ctx context.Context, cfg eval.Config, smokeRoot string) (int, error) {
	return runScoped(ctx, cfg, smokeRoot, "all")
}

func runScoped(ctx context.Context, cfg eval.Config, smokeRoot, scope string) (int, error) {
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
		for _, name := range plannedChecks(cfg.Freeze, scope) {
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
		remoteKhala := remoteHome(target) + "/khala"
		checker.remoteKVM(target)
		checker.remoteWorkerTools(target)
		checker.remoteMinio(target, baseURL)
		checker.remoteGit("worker_khala", target, remoteKhala, localKhala.Head, eval.KhalaBranch)
		checker.remoteGit("worker_firecracker", target, remoteHome(target)+"/firecracker", eval.FirecrackerHead, eval.FirecrackerBranch)
		checker.remoteRuntimeSnapshots(target, remoteKhala)
		checker.workerArtifacts(target, remoteKhala)
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
		checker.smokeEvidence(smokeRoot, scope)
		// Capture the Deployment metadata immediately before freezing.  This is
		// read-only and becomes the authoritative identity for final teardown;
		// a missing or malformed response must block acquisition.
		identityErr := captureCampaignActivatorBaseline(ctx, &rep)
		checker.record("activator_baseline", identityErr, "deployment/activator -n knative-serving")
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

func captureCampaignActivatorBaseline(ctx context.Context, rep *report) error {
	identity, err := captureActivatorIdentity(ctx)
	if err != nil {
		return err
	}
	if err := identity.Validate(); err != nil {
		return err
	}
	rep.ActivatorUID = identity.UID
	rep.ActivatorGeneration = identity.Generation
	return nil
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

func (c *checks) remoteRuntimeSnapshots(target, khalaRoot string) {
	path := khalaRoot + "/runtime/snapshots"
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

func (c *checks) workerArtifacts(target, khalaRoot string) {
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
		full := filepath.Join(khalaRoot, relative)
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

func (c *checks) smokeEvidence(root, scope string) {
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
			if lifecycleErr := validateLifecycleSmokeManifest(fields, "E2", filepath.Dir(path)); lifecycleErr != nil {
				err = errors.Join(err, fmt.Errorf("E2 %s: %w", fields["mode"], lifecycleErr))
			} else if checksumErr := validateArchivedOutputChecksums(filepath.Dir(path)); checksumErr != nil {
				err = errors.Join(err, fmt.Errorf("E2 %s: %w", fields["mode"], checksumErr))
			} else {
				e2[fields["mode"]] = true
			}
		case fields["experiment"] == "e3" && fields["end_scale"] == "1" && fields["claim_bearing"] == "false":
			if lifecycleErr := validateLifecycleSmokeManifest(fields, "E3", filepath.Dir(path)); lifecycleErr != nil {
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
	if scope == "e1" {
		e4InitialCleanup = true
	} else if len(e4InitialCleanups) != 1 {
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
		{"E1", e1, []string{"e1-smoke-4b", "e1-smoke-16mib", "e1-smoke-real"}},
	}
	if scope == "all" {
		wants = append(wants,
			struct {
				name string
				got  map[string]bool
				keys []string
			}{"E2", e2, []string{"invm-py", "nexus-py"}},
			struct {
				name string
				got  map[string]bool
				keys []string
			}{"E3", e3, []string{"invm-py", "nexus-py", "nexus-rdma-py"}},
			struct {
				name string
				got  map[string]bool
				keys []string
			}{"E4", e4, []string{"invm-py", "nexus-py"}},
		)
	}
	for _, want := range wants {
		for _, key := range want.keys {
			if !want.got[key] {
				err = errors.Join(err, fmt.Errorf("missing terminal %s smoke %s", want.name, key))
			}
		}
	}
	if scope == "all" && !e4InitialCleanup {
		err = errors.Join(err, fmt.Errorf("missing terminal E4 initial cleanup evidence"))
	}
	c.report.QualificationRoot = root
	c.report.QualificationScope = scope
	treeDigest, digestErr := directorySHA256(root)
	if digestErr != nil {
		err = errors.Join(err, fmt.Errorf("qualification tree digest: %w", digestErr))
	} else {
		c.report.QualificationSHA256 = treeDigest
	}
	checkName := "e1_e4_smoke_evidence"
	if scope == "e1" {
		checkName = "e1_smoke_evidence"
	}
	c.record(checkName, err, root)
}

func validateE1LifecycleSmokeManifest(fields map[string]string) error {
	if fields["manifest_version"] != "13" {
		return fmt.Errorf("manifest_version=%q, want 13", fields["manifest_version"])
	}
	if fields["cell_status_sequence"] != "started,complete" || fields["acquisition_retry"] != "false" ||
		fields["independent_continuation"] != "true" || fields["contamination_stop"] != "true" {
		return fmt.Errorf("incomplete E1 lifecycle contract")
	}
	if fields["fixture_setup_max_attempts"] != "2" || fields["cell_setup_max_attempts"] != "2" {
		return fmt.Errorf("unbounded E1 setup contract")
	}
	expected := map[string]map[string]string{
		"e1-smoke-4b": {
			"modes":              "invm-py invm-js invm-go nexus-sdk-py nexus-py nexus-js nexus-go",
			"synthetic_payloads": "4", "expected_cell_count": "7",
		},
		"e1-smoke-16mib": {
			"modes":              "invm-py invm-js invm-go nexus-sdk-py nexus-py nexus-js nexus-go",
			"synthetic_payloads": "16777216", "expected_cell_count": "7",
		},
		"e1-smoke-real": {
			"modes":     "invm-py nexus-sdk-py nexus-py nexus-rdma-py",
			"functions": "helloworld reducer", "expected_cell_count": "8",
		},
	}
	contract, ok := expected[fields["claim_id"]]
	if !ok {
		return fmt.Errorf("unexpected E1 smoke claim_id=%q", fields["claim_id"])
	}
	if fields["claim_id"] != "e1-smoke-real" && strings.Contains(fields["modes"], "rdma") {
		return fmt.Errorf("synthetic smoke must not contain RDMA modes")
	}
	for key, want := range contract {
		if fields[key] != want {
			return fmt.Errorf("%s=%q, want %q", key, fields[key], want)
		}
	}
	if fields["latency_iterations"] != "1" || fields["memory_iterations"] != "1" || fields["warm_invocations"] != "1" {
		return fmt.Errorf("E1 smoke requires latency=1, memory=1, and warm=1")
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
func validateLifecycleSmokeManifest(fields map[string]string, experiment, directory string) error {
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
	if fields["acquisition_started"] != "true" || fields["acquisition_retry"] != "false" || fields["independent_continuation"] != "true" {
		return fmt.Errorf("incomplete acquisition lifecycle contract")
	}
	if fields["snapshot_status"] != "0" {
		return fmt.Errorf("snapshot_status=%q, want 0", fields["snapshot_status"])
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
	if err := validateManifestFileHash(fields, "evidence_validation_sha256", directory, "evidence-validation.txt"); err != nil {
		return err
	}
	switch experiment {
	case "E2":
		if fields["evidence_status"] != "0" || fields["admission_status"] != "0" {
			return fmt.Errorf("E2 evidence/admission status is not PASS")
		}
		expected, err := positiveManifestInt(fields, "admission_expected_replicas")
		if err != nil {
			return err
		}
		functions, err := positiveManifestInt(fields, "admission_function_count")
		if err != nil {
			return err
		}
		aggregateExpected, err := positiveManifestInt(fields, "admission_aggregate_expected_replicas")
		if err != nil {
			return err
		}
		aggregateReady, err := positiveManifestInt(fields, "admission_aggregate_ready_replicas")
		if err != nil {
			return err
		}
		if aggregateExpected != expected*functions || aggregateReady != aggregateExpected {
			return fmt.Errorf("E2 admission aggregate mismatch: replicas=%d functions=%d expected=%d ready=%d", expected, functions, aggregateExpected, aggregateReady)
		}
		if fields["snapshot_workload_count"] != "1" {
			return fmt.Errorf("snapshot_workload_count=%q, want 1", fields["snapshot_workload_count"])
		}
		if err := validateManifestFileHash(fields, "admission_evidence_sha256", directory, "admission-validation.txt"); err != nil {
			return err
		}
		if err := validateManifestFileHash(fields, "admission_readiness_sha256", directory, "admission.csv"); err != nil {
			return err
		}
	case "E3":
		if fields["evidence_status"] != "0" || fields["scientific_status"] != "ACCEPTED" {
			return fmt.Errorf("E3 scientific evidence is not accepted")
		}
		successes, err := positiveManifestInt(fields, "success_count")
		if err != nil {
			return err
		}
		failures, err := nonNegativeManifestInt(fields, "failure_count")
		if err != nil {
			return err
		}
		if failures*100 > (successes+failures)*5 {
			return fmt.Errorf("E3 failure fraction exceeds 5%%")
		}
		fraction, err := strconv.ParseFloat(fields["failure_fraction"], 64)
		if err != nil {
			return fmt.Errorf("failure_fraction=%q is malformed", fields["failure_fraction"])
		}
		expectedFraction := float64(failures) / float64(successes+failures)
		if fraction < expectedFraction-1e-12 || fraction > expectedFraction+1e-12 {
			return fmt.Errorf("failure_fraction=%q disagrees with success/failure counts", fields["failure_fraction"])
		}
		if fields["snapshot_workload_count"] != "10" {
			return fmt.Errorf("snapshot_workload_count=%q, want 10", fields["snapshot_workload_count"])
		}
	default:
		return fmt.Errorf("unknown lifecycle smoke experiment %q", experiment)
	}
	return nil
}

func positiveManifestInt(fields map[string]string, key string) (int, error) {
	value, err := strconv.Atoi(fields[key])
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s=%q, want a positive integer", key, fields[key])
	}
	return value, nil
}

func nonNegativeManifestInt(fields map[string]string, key string) (int, error) {
	value, err := strconv.Atoi(fields[key])
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s=%q, want a non-negative integer", key, fields[key])
	}
	return value, nil
}

func validateManifestFileHash(fields map[string]string, key, directory, name string) error {
	expected := fields[key]
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("%s is missing or malformed", key)
	}
	actual, err := eval.SHA256File(filepath.Join(directory, name))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if actual != expected {
		return fmt.Errorf("%s digest mismatch", name)
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
	actualFiles := map[string]bool{}
	if err := filepath.WalkDir(directory, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(directory, candidate)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if relative != "manifest.txt" && relative != "archived-output-checksums.csv" {
			actualFiles[relative] = true
		}
		return nil
	}); err != nil {
		return err
	}
	if len(actualFiles) != len(seen) {
		return fmt.Errorf("%s is incomplete: listed=%d actual=%d", path, len(seen), len(actualFiles))
	}
	for relative := range actualFiles {
		if !seen[relative] {
			return fmt.Errorf("%s omits %q", path, relative)
		}
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

func plannedChecks(freeze bool, scope string) []string {
	values := []string{"local_git", "kubernetes_nodes", "kubernetes_topology", "minio_loader", "kubernetes_workloads", "prometheus_api_ready", "worker_kvm", "worker_tools", "worker_flamegraph", "worker_minio", "worker_runtime_snapshots", "deployed_git", "unified_rootfs", "artifact_hashes", "rdma"}
	if freeze {
		smokeCheck := "e1_e4_smoke_evidence"
		if scope == "e1" {
			smokeCheck = "e1_smoke_evidence"
		}
		values = append(values, smokeCheck, "activator_baseline")
	}
	return values
}

func fail(message string) { fmt.Fprintln(os.Stderr, "preflight:", message); os.Exit(2) }
