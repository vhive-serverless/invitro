package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/loader/pkg/workload/proto"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	Command         = flag.String("command", "deploy", "Command to execute: deploy or clean")
	Mode            = flag.String("mode", "", "Experiment mode: invm-py, nexus-go, or nexus-rdma")
	DryRun          = flag.Bool("dry-run", false, "Print the resolved deployment plan without side effects")
	CorePoolPolicy  = flag.String("core-pool-policy", "", "Core pool policy: baseline, l-sep, or l-shared")
	Implementation  = flag.String("impl", "go", "Implementation to use: go or cpp")
	RemoveSnapshots = flag.Bool("remove-snapshots", false, "Whether to remove existing snapshots before deploying Khala")
	CorePoolNode    = flag.String("corepool-node", "", "Node to set manual core pool size when using 'set-corepool' command")
	CorePool        = flag.String("corepool-size", "", "Manual core pool size to set when using 'set-corepool' command")
	StreamSlots     = flag.Int("stream-slots", 4, "Shared-memory stream slot count")
	StreamCapacity  = flag.Int("stream-capacity", 256*1024, "Shared-memory capacity per direction per slot")
	Debug           = flag.Bool("debug", false, "Enable debug mode")
)

const (
	ModeInVMPy    = "invm-py"
	ModeNexusGo   = "nexus-go"
	ModeNexusRDMA = "nexus-rdma"
)

var matchedWorkloads = []string{"pyaesserve", "mapper", "reducer"}

type ExperimentMode struct {
	Name             string   `json:"mode"`
	Workloads        []string `json:"workloads"`
	TCPTransport     string   `json:"tcp_transport"`
	BackendTransport string   `json:"backend_transport"`
	SetNexusSDK      bool     `json:"set_nexus_sdk"`
	SetNexusRPC      bool     `json:"set_nexus_rpc"`
	WithRDMA         bool     `json:"with_rdma"`
	StreamSlots      int      `json:"stream_slots"`
	StreamCapacity   int      `json:"stream_capacity"`
}

type DryRunPlan struct {
	ExperimentMode
	DeploymentCommand string   `json:"deployment_command"`
	Snapshots         []string `json:"snapshots"`
	CleanupRDMA       bool     `json:"cleanup_rdma"`
}

type CleanupDryRunPlan struct {
	Mode        string `json:"mode"`
	CleanupRDMA bool   `json:"cleanup_rdma"`
}

func main() {
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.999",
		FullTimestamp:   true,
	})
	if err := runCommand(); err != nil {
		log.Error(err)
		os.Exit(1)
	}
}

func runCommand() error {
	if err := validateLocalFlags(*Command, *CorePoolPolicy, *Implementation); err != nil {
		return err
	}

	var mode ExperimentMode
	var err error
	if *Command != "set-corepool" {
		if *Command == "clean" {
			mode, err = resolveCleanupMode(*Mode)
		} else {
			mode, err = resolveExperimentMode(*Mode, *StreamSlots, *StreamCapacity)
		}
		if err != nil {
			return err
		}
	}
	if *DryRun && *Command != "set-corepool" {
		var plan any
		if *Command == "clean" {
			plan = CleanupDryRunPlan{Mode: mode.Name, CleanupRDMA: mode.WithRDMA}
		} else {
			plan = buildDryRunPlan(mode, *CorePoolPolicy, *Implementation, *Debug)
		}
		encoded, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(encoded))
		return nil
	}
	if *DryRun {
		return fmt.Errorf("--dry-run is supported only for mode-aware deploy, clean, and create-snapshots commands")
	}

	workerNodeSetup, err := getWorkerNodesFn()
	if err != nil {
		return fmt.Errorf("read worker node setup: %w", err)
	}

	switch *Command {
	case "deploy":
		log.Infof("Deploying Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
		if err := DeployKhala(workerNodeSetup, *CorePoolPolicy, *Implementation, mode, *Debug); err != nil {
			return err
		}
		if mode.WithRDMA {
			if err := DeployRDMAStorage(workerNodeSetup); err != nil {
				return err
			}
		}
	case "clean":
		log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
		return cleanKhalaFn(workerNodeSetup, *RemoveSnapshots, mode.WithRDMA)
	case "create-snapshots":
		log.Infof("Creating snapshots on worker nodes: %v", workerNodeSetup.WorkerNodes)
		return CreateSnapshots(workerNodeSetup, mode)
	case "set-corepool":
		log.Infof("Setting manual core pool size to %s on worker nodes: %v", *CorePool, workerNodeSetup.WorkerNodes)
		if *CorePoolNode == "" || *CorePool == "" {
			return fmt.Errorf("both --core-pool-node and --core-pool-size must be specified for 'set-corepool' command")
		}
		return SetManualCorePool(*CorePoolNode, *CorePool, workerNodeSetup)
	default:
		return fmt.Errorf("unknown command: %s", *Command)
	}
	return nil
}

func validateLocalFlags(command, corePoolPolicy, implementation string) error {
	switch command {
	case "deploy", "clean", "create-snapshots", "set-corepool":
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
	if command == "clean" {
		return nil
	}
	switch corePoolPolicy {
	case "", "corepool_freq_static", "corepool_freq_dynamic":
	default:
		return fmt.Errorf("unknown core pool policy: %s", corePoolPolicy)
	}
	if implementation != "go" && implementation != "cpp" {
		return fmt.Errorf("unknown implementation: %s", implementation)
	}
	if command != "set-corepool" && implementation != "go" {
		return fmt.Errorf("evaluated modes require --impl=go; got %q", implementation)
	}
	return nil
}

func resolveExperimentMode(name string, streamSlots, streamCapacity int) (ExperimentMode, error) {
	if streamSlots <= 0 || streamCapacity <= 0 {
		return ExperimentMode{}, fmt.Errorf("stream slots and capacity must be positive")
	}
	mode := ExperimentMode{Name: name, StreamSlots: streamSlots, StreamCapacity: streamCapacity}
	switch name {
	case ModeInVMPy:
		mode.Workloads = append([]string(nil), matchedWorkloads...)
		mode.TCPTransport = "guest"
		mode.BackendTransport = "none"
	case ModeNexusGo:
		if err := validateStreamLayout(streamSlots, streamCapacity); err != nil {
			return ExperimentMode{}, err
		}
		mode.Workloads = goWorkloads()
		mode.TCPTransport = "guest"
		mode.BackendTransport = "stream"
		mode.SetNexusSDK = true
		mode.SetNexusRPC = true
	case ModeNexusRDMA:
		mode.Workloads = goWorkloads()
		mode.TCPTransport = "guest"
		mode.BackendTransport = "rdma"
		mode.SetNexusSDK = true
		mode.SetNexusRPC = true
		mode.WithRDMA = true
	default:
		return ExperimentMode{}, fmt.Errorf("invalid --mode %q: expected %s, %s, or %s", name, ModeInVMPy, ModeNexusGo, ModeNexusRDMA)
	}
	return mode, nil
}

// resolveCleanupMode intentionally validates only the experiment identity and
// whether its teardown includes RDMA storage. Cleanup must remain usable after
// a failed or historical deployment even when its stream layout flags are no
// longer valid under the current deploy/create rules.
func resolveCleanupMode(name string) (ExperimentMode, error) {
	mode := ExperimentMode{Name: name}
	switch name {
	case ModeInVMPy, ModeNexusGo:
		return mode, nil
	case ModeNexusRDMA:
		mode.WithRDMA = true
		return mode, nil
	default:
		return ExperimentMode{}, fmt.Errorf("invalid --mode %q: expected %s, %s, or %s", name, ModeInVMPy, ModeNexusGo, ModeNexusRDMA)
	}
}

const maxSharedMemoryBytes = 16 * 1024 * 1024

func validateStreamLayout(slots, capacity int) error {
	if slots < 2 {
		return fmt.Errorf("stream mode requires at least 2 slots (one outbound reservation plus one allocatable slot)")
	}
	if slots > maxSharedMemoryBytes || capacity > maxSharedMemoryBytes {
		return fmt.Errorf("stream slots and capacity must each fit within the %d-byte shared-memory backing", maxSharedMemoryBytes)
	}
	// Mirrors nexusstream.CalculateLayout: a 64-byte superblock and, per slot,
	// 64 bytes of slot metadata plus two 128-byte ring headers and payloads.
	ringSpan := uint64(128) + uint64(capacity)
	ringSpan = (ringSpan + 63) &^ uint64(63)
	logical := uint64(64) + uint64(slots)*(uint64(64)+2*ringSpan)
	if logical > maxSharedMemoryBytes {
		return fmt.Errorf("stream layout requires %d bytes, exceeding %d-byte shared-memory backing", logical, maxSharedMemoryBytes)
	}
	return nil
}

func goWorkloads() []string {
	result := make([]string, 0, len(matchedWorkloads))
	for _, workload := range matchedWorkloads {
		result = append(result, "go"+workload)
	}
	return result
}

func buildDeploymentCommand(corePoolPolicy, implementation string, mode ExperimentMode, debug bool) string {
	command := "cd ~/khala && sudo ./bin/kn-integration --pool-size=20"
	command += " --impl=" + implementation
	if corePoolPolicy != "" {
		command += " --corepool=" + corePoolPolicy
	}
	command += fmt.Sprintf(" --tcp-transport=%s --backend-transport=%s", mode.TCPTransport, mode.BackendTransport)
	command += fmt.Sprintf(" --set-nexus-sdk=%t --set-nexus-rpc=%t --with-rdma=%t", mode.SetNexusSDK, mode.SetNexusRPC, mode.WithRDMA)
	command += fmt.Sprintf(" --stream-slots=%d --stream-capacity=%d --debug=%t", mode.StreamSlots, mode.StreamCapacity, debug)
	return command
}

func buildSnapshotNames(mode ExperimentMode) []string {
	result := make([]string, 0, len(mode.Workloads))
	for _, workload := range mode.Workloads {
		suffix := ""
		if mode.SetNexusSDK {
			suffix += "-s3"
		}
		if mode.SetNexusRPC {
			suffix += "-rpc"
		}
		if mode.BackendTransport == "stream" {
			suffix += "-stream"
		}
		result = append(result, workload+suffix+"-0")
	}
	return result
}

func buildDryRunPlan(mode ExperimentMode, corePoolPolicy, implementation string, debug bool) DryRunPlan {
	return DryRunPlan{
		ExperimentMode:    mode,
		DeploymentCommand: buildDeploymentCommand(corePoolPolicy, implementation, mode, debug),
		Snapshots:         buildSnapshotNames(mode),
		CleanupRDMA:       mode.WithRDMA,
	}
}

type WorkerNodeSetup struct {
	WorkerNodes  []string `json:"worker_nodes"`
	StorageNodes []string `json:"storage_nodes"`
}

var (
	serverExecFn          = loaderUtils.ServerExec
	localCommandFn        = runLocalCommand
	getWorkerNodesFn      = getWorkerNodes
	cleanKhalaFn          = CleanKhala
	setDefaultCorePoolFn  = SetDefaultCorePool
	createSnapshotsNodeFn = createSnapshotsOnNode
	sleepFn               = time.Sleep
)

func runLocalCommand(command string) (string, error) {
	output, err := exec.Command("bash", "-c", command).CombinedOutput()
	return string(output), err
}

type errorCollector struct {
	mu   sync.Mutex
	errs []error
}

func (c *errorCollector) add(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.errs = append(c.errs, err)
}

func (c *errorCollector) joined() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return errors.Join(c.errs...)
}

func getWorkerNodes() (WorkerNodeSetup, error) {
	configFile, err := os.ReadFile("worker_node.json")
	if err != nil {
		return WorkerNodeSetup{}, err
	}
	var workerNodeSetup WorkerNodeSetup
	if err := json.Unmarshal(configFile, &workerNodeSetup); err != nil {
		return WorkerNodeSetup{}, err
	}
	if len(workerNodeSetup.WorkerNodes) != len(workerNodeSetup.StorageNodes) {
		return WorkerNodeSetup{}, fmt.Errorf("number of worker nodes and storage nodes must be the same")
	}
	return workerNodeSetup, nil
}

func DeployKhala(workerNodeSetup WorkerNodeSetup, corePoolPolicy string, implementation string, mode ExperimentMode, debug bool) error {
	output, err := localCommandFn("cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	if err != nil {
		return fmt.Errorf("prepare MinIO objects: %w, output: %s", err, output)
	}

	deploymentCmd := buildDeploymentCommand(corePoolPolicy, implementation, mode, debug)

	commands := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`tmux new-session -d -s kn-integration`,
	}

	var wg sync.WaitGroup
	var workerErrors errorCollector
	for nodeIndex, workerNode := range workerNodeSetup.WorkerNodes {
		wg.Add(1)
		go func(node string, idx int) {
			defer wg.Done()

			nodeCmd := fmt.Sprintf("%s --storage-ip=%s:10191", deploymentCmd, workerNodeSetup.StorageNodes[idx])

			nodeCommands := append([]string(nil), commands...)
			nodeCommands = append(nodeCommands, fmt.Sprintf(`tmux send-keys -t kn-integration "%s" C-m`, nodeCmd))

			for _, cmd := range nodeCommands {
				_, err := serverExecFn(node, cmd)
				if err != nil {
					workerErrors.add(fmt.Errorf("worker %s command %q: %w", node, cmd, err))
					return
				}
			}
			log.Infof("Khala deployed on worker node %s", node)
		}(workerNode, nodeIndex)
	}
	wg.Wait()
	if err := workerErrors.joined(); err != nil {
		return fmt.Errorf("deploy Khala: %w", err)
	}
	sleepFn(10 * time.Second)

	var corePoolErrors errorCollector
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		err := setDefaultCorePoolFn(workerNode)
		if err != nil {
			log.Errorf("Failed to set default core pool on worker node %s: %v", workerNode, err)
			corePoolErrors.add(fmt.Errorf("set default core pool on %s: %w", workerNode, err))
		}
	}
	return corePoolErrors.joined()
}

func DeployRDMAStorage(workerNodeSetup WorkerNodeSetup) error {
	baseCommands := []string{
		`sudo pkill --signal INT s3-rdma-server 2>/dev/null || true`,
		`tmux kill-session -t s3-rdma-server 2>/dev/null || true`,
		`tmux new-session -d -s s3-rdma-server`,
	}
	baseDeploymentCmd := "cd ~/rdma-demo && sudo ./s3-rdma-server --payload-root=assets --enable-rdma-zcopy=true"

	log.Infof("Deploying RDMA storage on worker nodes: %v", workerNodeSetup.StorageNodes)

	var wg sync.WaitGroup
	var workerErrors errorCollector
	for _, storageNode := range workerNodeSetup.StorageNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			nodeCmd := fmt.Sprintf("%s --tcp-listen=%s:10090 --rdma-zcopy-listen=%s:10191", baseDeploymentCmd, node, node)
			nodeCommands := append([]string(nil), baseCommands...)
			nodeCommands = append(nodeCommands, fmt.Sprintf(`tmux send-keys -t s3-rdma-server "%s" C-m`, nodeCmd))
			for _, cmd := range nodeCommands {
				_, err := serverExecFn(node, cmd)
				if err != nil {
					workerErrors.add(fmt.Errorf("RDMA storage %s command %q: %w", node, cmd, err))
					return
				}
			}
			log.Infof("RDMA storage deployed on storage node %s", node)
		}(storageNode)
	}

	wg.Wait()
	return workerErrors.joined()
}

func CleanKhala(workerNodeSetup WorkerNodeSetup, removeSnapshots bool, withRDMA bool) error {
	log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
	commands := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`sudo rm -rf ~/khala/runtime/overlayfs/*.overlay`,
		`sudo rm -rf ~/khala/runtime/logs/*.log`,
		`sudo rm -rf ~/khala/runtime/metrics/*.metrics`,
		`sudo rm -rf ~/khala/runtime/uffd_sock/*.sock`,
		`bash -c 'cd ~/khala && bash cleanup_worker.sh'`,
	}
	if removeSnapshots {
		commands = append(commands,
			`sudo rm -rf ~/khala/runtime/snapshots/*.snapshot`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.mem`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.trace`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.ws`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.overlay`,
		)
	}

	var wg sync.WaitGroup
	var khalaDied atomic.Bool
	var cleanupErrors errorCollector
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			conn, err := grpc.NewClient(node+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Errorf("Failed to connect to nexus endpoint %s: %v", node, err)
				khalaDied.Store(true)
				cleanupErrors.add(fmt.Errorf("connect to Khala on %s: %w", node, err))
			} else {
				defer conn.Close()
				client := proto.NewKhalaKnativeIntegrationClient(conn)

				destroyAllCtx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
				defer cancel()
				_, err = client.DestroyAll(destroyAllCtx, &proto.DestroyAllRequest{DestroyAll: true})
				if err != nil {
					log.Errorf("Failed to destroy all on nexus endpoint %s: %v", node, err)
					khalaDied.Store(true)
					cleanupErrors.add(fmt.Errorf("destroy all on %s: %w", node, err))
				}
			}

			for _, cmd := range commands {
				_, err := serverExecFn(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on nexus endpoint %s: %v", cmd, node, err)
					cleanupErrors.add(fmt.Errorf("cleanup %s command %q: %w", node, cmd, err))
				}
			}

		}(workerNode)
	}
	wg.Wait()

	if withRDMA {
		cleanupErrors.add(CleanupRDMAStorage(workerNodeSetup))
	}

	out, err := serverExecFn("10.0.1.1", "bash -c 'cd ~/loader && bash cleanup_etcd.sh'")
	if err != nil {
		log.Errorf("Failed to clean etcd: %v, output: %s", err, out)
		cleanupErrors.add(fmt.Errorf("clean etcd: %w", err))
	}

	cmd := exec.Command("bash", "-c", "cd ~/loader && make clean && sleep 1 && make clean")
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clean loader: %v, output: %s", err, string(outBytes))
		cleanupErrors.add(fmt.Errorf("clean loader: %w", err))
	}

	clusterCommands := []string{
		"kubectl rollout restart daemonset calico-node -n kube-system",
		"kubectl rollout status daemonset calico-node -n kube-system",
		"sleep 10",
		"kubectl rollout restart deployment calico-kube-controllers -n kube-system",
		"kubectl rollout status deployment calico-kube-controllers -n kube-system",
		"sleep 10",
		"kubectl rollout restart daemonset speaker -n metallb-system",
		"kubectl rollout status daemonset speaker -n metallb-system",
		"sleep 10",
	}
	log.Infof("Khala appears to have died on one or more worker nodes, restarting calico")
	for _, command := range clusterCommands {
		cmd := exec.Command("bash", "-c", command)
		outBytes, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(outBytes))
			cleanupErrors.add(fmt.Errorf("restart cluster component: %w", err))
		}
	}
	activatorCommands := []string{
		"kubectl rollout restart -n knative-serving deployment/activator",
		"kubectl rollout status -n knative-serving deployment/activator",
		"sleep 10",
	}
	log.Infof("Restarting knative activator")
	for _, command := range activatorCommands {
		cmd := exec.Command("bash", "-c", command)
		outBytes, err := cmd.CombinedOutput()
		if err != nil {
			log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(outBytes))
			cleanupErrors.add(fmt.Errorf("restart activator: %w", err))
		}
	}

	log.Infof("Cleaning up minio")
	if khalaDied.Load() {
		out, err = serverExecFn("10.0.1.1", "bash -c 'source /etc/profile && cd ~/loader/scripts/setup && go run setup.go --setup-type=cleanup_minio --config=node_setup.json'")
		if err != nil {
			log.Errorf("Failed to clean minio: %v, output: %s", err, out)
			cleanupErrors.add(fmt.Errorf("clean MinIO: %w", err))
		}
		time.Sleep(10 * time.Second)
		out, err = serverExecFn("10.0.1.1", "bash -c 'source /etc/profile && cd ~/loader/scripts/setup && go run setup.go --setup-type=redeploy_minio --config=node_setup.json'")
		if err != nil {
			log.Errorf("Failed to redeploy minio: %v, output: %s", err, out)
			cleanupErrors.add(fmt.Errorf("redeploy MinIO: %w", err))
		}

		time.Sleep(60 * time.Second)
	}

	cmd = exec.Command("bash", "-c", "cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	outBytes, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to cleanup minio: %v, output: %s", err, string(outBytes))
		cleanupErrors.add(fmt.Errorf("prepare MinIO objects: %w", err))
	}

	log.Infof("Khala cleaned on all worker nodes")
	return cleanupErrors.joined()
}

func CleanupRDMAStorage(workerNodeSetup WorkerNodeSetup) error {
	commands := []string{
		`sudo pkill --signal INT s3-rdma-server 2>/dev/null || true`,
		`tmux kill-session -t s3-rdma-server 2>/dev/null || true`,
	}

	log.Infof("Cleaning RDMA storage on worker nodes: %v", workerNodeSetup.StorageNodes)
	var wg sync.WaitGroup
	var cleanupErrors errorCollector
	for _, storageNode := range workerNodeSetup.StorageNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			for _, cmd := range commands {
				_, err := serverExecFn(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on storage node %s: %v", cmd, node, err)
					cleanupErrors.add(fmt.Errorf("cleanup RDMA storage %s command %q: %w", node, cmd, err))
				}
			}
		}(storageNode)
	}
	wg.Wait()
	return cleanupErrors.joined()
}

func CreateSnapshots(workerNodeSetup WorkerNodeSetup, mode ExperimentMode) error {
	workloadList := buildSnapshotNames(mode)

	var wg sync.WaitGroup
	var workerErrors errorCollector
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			if err := createSnapshotsNodeFn(node, workloadList); err != nil {
				workerErrors.add(err)
			}
		}(workerNode)
	}
	wg.Wait()
	return workerErrors.joined()
}

func createSnapshotsOnNode(node string, workloads []string) error {
	conn, err := grpc.NewClient(node+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect to Khala on %s: %w", node, err)
	}
	defer conn.Close()
	client := proto.NewKhalaKnativeIntegrationClient(conn)
	for _, workload := range workloads {
		if _, err := client.CreateSnapshot(context.Background(), &proto.CreateSnapshotRequest{Workload: workload}); err != nil {
			return fmt.Errorf("create snapshot %s on %s: %w", workload, node, err)
		}
		log.Infof("Snapshot created for function %s on nexus endpoint %s", workload, node)
	}
	return nil
}

func SetManualCorePool(node string, corePoolSetting string, workerNodeSetup WorkerNodeSetup) error {
	// parse core pool setting
	// 'C:18@2.1,IO:10@1.0'
	// means set core pool for CPU-intensive functions to 18 with frequency scaling factor 2.1
	// and for IO-intensive functions to 10 with frequency scaling factor 1.0

	//parse corePoolSize
	conn, err := grpc.NewClient(node+":8002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Errorf("Failed to connect to hardware manager on node %s: %v", node, err)
		return err
	}
	defer conn.Close()
	client := proto.NewHardwareManagerClient(conn)

	// set core pool
	corePoolList := corePoolParser(corePoolSetting)
	for i := range corePoolList {
		corePool := &corePoolList[i]
		_, err = client.SetCorePool(context.Background(), corePool)
		if err != nil {
			log.Errorf("Failed to set core pool on node %s: %v", node, err)
			return err
		} else {
			log.Infof("Set core pool %s on node %s", corePool.GetName(), node)
		}
	}
	return nil
}

func corePoolParser(corePoolSetting string) []proto.CorePool {
	//C:18@2.1,IO:10@1.0
	//corepoolname:coresize@corefreq
	// C should be from 0 to 17 uint32[0,1,...,17]
	// IO should be from 18 to 27 uint32[18,19,...,27]
	// C freq should be [2100000,2100000,...,2100000] in kHz
	// IO freq should be [1000000,1000000,...,1000000] in kHz

	var computeCoreCount int
	var ioCoreCount int
	var computeCoreFreq int
	var ioCoreFreq int

	corePoolSettings := strings.Split(corePoolSetting, ",")

	for _, setting := range corePoolSettings {
		parts := strings.Split(setting, ":")
		if len(parts) != 2 {
			log.Fatalf("Invalid core pool setting: %s", setting)
		}
		poolName := parts[0]
		sizeFreq := strings.Split(parts[1], "@")
		if len(sizeFreq) != 2 {
			log.Fatalf("Invalid core pool size and frequency: %s", parts[1])
		}
		size, err := strconv.Atoi(sizeFreq[0])
		if err != nil {
			log.Fatalf("Invalid core pool size: %s", sizeFreq[0])
		}
		freqFloat, err := strconv.ParseFloat(sizeFreq[1], 64)
		if err != nil {
			log.Fatalf("Invalid core pool frequency: %s", sizeFreq[1])
		}
		freq := uint32(freqFloat * 1e6) // convert GHz to kHz

		switch poolName {
		case "C":
			computeCoreCount = size
			computeCoreFreq = int(freq)
		case "IO":
			ioCoreCount = size
			ioCoreFreq = int(freq)
		default:
			log.Fatalf("Unknown core pool name: %s", poolName)
		}
	}

	corePoolList := []proto.CorePool{
		getCorePool("empty", 0, 28, 2100000, false), // empty core pool to avoid errors
		getCorePool("nexus", ioCoreCount, 0, ioCoreFreq, true),
		getCorePool("firecracker", computeCoreCount, ioCoreCount, computeCoreFreq, true),
	}

	return corePoolList
}

func getCorePool(name string, nCore int, fromCore int, coreFreq int, reuseCgroup bool) proto.CorePool {
	if fromCore == 28 {
		fromCore = 0
	}
	coreList := make([]uint32, nCore)
	for i := 0; i < nCore; i++ {
		coreList[i] = uint32(fromCore + i)
	}
	coreFreqList := make([]uint32, nCore)
	for i := 0; i < nCore; i++ {
		coreFreqList[i] = uint32(coreFreq)
	}
	return proto.CorePool{
		Name:        name,
		CoreList:    coreList,
		CoreFreq:    coreFreqList,
		ReuseCgroup: reuseCgroup,
	}
}

func SetDefaultCorePool(node string) error {
	// parse core pool setting
	// 'C:18@2.1,IO:10@1.0'
	// means set core pool for CPU-intensive functions to 18 with frequency scaling factor 2.1
	// and for IO-intensive functions to 10 with frequency scaling factor 1.0

	//parse corePoolSize
	conn, err := grpc.NewClient(node+":8002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Errorf("Failed to connect to hardware manager on node %s: %v", node, err)
		return err
	}
	defer conn.Close()
	client := proto.NewHardwareManagerClient(conn)

	// set core pool
	corePoolList := []proto.CorePool{
		getCorePool("empty", 4, 0, 2100000, true), // empty core pool to avoid errors
		getCorePool("nexus", 28, 0, 2100000, true),
		getCorePool("firecracker", 28, 0, 2100000, true),
	}
	for i := range corePoolList {
		corePool := &corePoolList[i]
		_, err = client.SetCorePool(context.Background(), corePool)
		if err != nil {
			log.Errorf("Failed to set core pool on node %s: %v", node, err)
			return err
		} else {
			log.Infof("Set core pool %s on node %s", corePool.GetName(), node)
		}
	}

	return nil
}
