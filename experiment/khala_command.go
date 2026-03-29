package main

import (
	"context"
	"encoding/json"
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
	CorePoolPolicy  = flag.String("core-pool-policy", "", "Core pool policy: baseline, l-sep, or l-shared")
	Implementation  = flag.String("impl", "go", "Implementation to use: go or cpp")
	RemoveSnapshots = flag.Bool("remove-snapshots", false, "Whether to remove existing snapshots before deploying Khala")
	CorePoolNode    = flag.String("corepool-node", "", "Node to set manual core pool size when using 'set-corepool' command")
	CorePool        = flag.String("corepool-size", "", "Manual core pool size to set when using 'set-corepool' command")
	SetNexusSDK     = flag.Bool("set-nexus-sdk", false, "Whether to set Nexus SDK environment variable for worker nodes")
	SetNexusRPC     = flag.Bool("set-nexus-rpc", false, "Whether to set Nexus RPC environment variable for worker nodes")
	WithRDMA        = flag.Bool("with-rdma", false, "Whether to deploy RDMA storage along with Khala")
	Debug           = flag.Bool("debug", false, "Enable debug mode")

	WorkloadList = []string{"helloworld", "chameleonserve", "cnnserve", "imageresize", "lrserving", "mapper", "pyaesserve", "reducer", "rnnserve", "streducer", "sttrainer"}
	// WorkloadList = []string{"helloworld"}
)

func main() {
	// unmarshall config file
	flag.Parse()

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: "2006-01-02T15:04:05.999",
		FullTimestamp:   true,
	})

	workerNodeSetup, err := getWorkerNodes()
	if err != nil {
		log.Fatalf("Failed to read worker node setup: %v", err)
	}

	switch *CorePoolPolicy {
	case "":
		log.Infof("Using baseline core pool policy")
	case "corepool_freq_static":
		log.Infof("Using static core pool policy")
	case "corepool_freq_dynamic":
		log.Infof("Using dynamic core pool policy")
	default:
		log.Fatalf("Unknown core pool policy: %s", *CorePoolPolicy)
	}

	if *Implementation != "go" && *Implementation != "cpp" {
		log.Fatalf("Unknown implementation: %s", *Implementation)
	}

	switch *Command {
	case "deploy":
		log.Infof("Deploying Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
		DeployKhala(workerNodeSetup, *CorePoolPolicy, *Implementation, *WithRDMA, *Debug)
		if *WithRDMA {
			DeployRDMAStorage(workerNodeSetup)
		}
	case "clean":
		log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
		CleanKhala(workerNodeSetup, *RemoveSnapshots)
	case "create-snapshots":
		log.Infof("Creating snapshots on worker nodes: %v", workerNodeSetup.WorkerNodes)
		CreateSnapshots(workerNodeSetup)
	case "set-corepool":
		log.Infof("Setting manual core pool size to %s on worker nodes: %v", *CorePool, workerNodeSetup.WorkerNodes)
		if *CorePoolNode == "" || *CorePool == "" {
			log.Fatalf("Both --core-pool-node and --core-pool-size must be specified for 'set-corepool' command")
		}
		SetManualCorePool(*CorePoolNode, *CorePool, workerNodeSetup)
	default:
		log.Fatalf("Unknown command: %s", *Command)
	}

}

type WorkerNodeSetup struct {
	WorkerNodes  []string `json:"worker_nodes"`
	StorageNodes []string `json:"storage_nodes"`
}

func getWorkerNodes() (WorkerNodeSetup, error) {
	configFile, err := os.ReadFile("worker_node.json")
	if err != nil {
		return WorkerNodeSetup{}, err
	}
	var workerNodeSetup WorkerNodeSetup
	err = json.Unmarshal(configFile, &workerNodeSetup)
	if err != nil {
		return WorkerNodeSetup{}, err
	}
	if len(workerNodeSetup.WorkerNodes) != len(workerNodeSetup.StorageNodes) {
		return WorkerNodeSetup{}, fmt.Errorf("number of worker nodes and storage nodes must be the same")
	}
	return workerNodeSetup, nil
}

func DeployKhala(workerNodeSetup WorkerNodeSetup, corePoolPolicy string, implementation string, withRDMA bool, debug bool) error {
	// 1. cleanup minio
	log.Infof("Cleaning up minio")
	cmd := exec.Command("bash", "-c", "cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to cleanup minio: %v, output: %s", err, string(output))
		return err
	}

	deploymentCmd := "cd ~/khala && sudo ./bin/kn-integration --pool-size=20"
	deploymentCmd += " --impl=" + implementation
	if corePoolPolicy != "" {
		deploymentCmd += " --corepool=" + corePoolPolicy
	}

	if *SetNexusSDK {
		deploymentCmd += " --set-nexus-sdk=true"
	} else {
		deploymentCmd += " --set-nexus-sdk=false"
	}

	if *SetNexusRPC {
		deploymentCmd += " --set-nexus-rpc=true"
	} else {
		deploymentCmd += " --set-nexus-rpc=false"
	}

	if withRDMA {
		deploymentCmd += " --with-rdma=true"
	} else {
		deploymentCmd += " --with-rdma=false"
	}

	CommandList := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`tmux new-session -d -s kn-integration`,
	}

	// 2. deploy khala on all worker nodes
	log.Infof("Deploying Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
	var wg sync.WaitGroup
	for nodeIndex, workerNode := range workerNodeSetup.WorkerNodes {
		go func(node string, nodeIndex int) {
			defer wg.Done()
			deploymentCmd += " --storage-ip=" + workerNodeSetup.StorageNodes[nodeIndex] + ":10191"
			CommandList = append(CommandList, fmt.Sprintf(`tmux send-keys -t kn-integration "%s" C-m`, deploymentCmd))
			for _, cmd := range CommandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on worker node %s: %v", cmd, workerNode, err)
				}
			}
			log.Infof("Khala deployed on worker node %s", workerNode)
		}(workerNode, nodeIndex)
		wg.Add(1)
	}
	wg.Wait()
	time.Sleep(10 * time.Second)

	for _, workerNode := range workerNodeSetup.WorkerNodes {
		err := SetDefaultCorePool(workerNode)
		if err != nil {
			log.Errorf("Failed to set default core pool on worker node %s: %v", workerNode, err)
			return err
		}
	}

	return nil
}

func DeployRDMAStorage(workerNodeSetup WorkerNodeSetup) {
	CommandList := []string{
		`sudo pkill --signal INT s3-rdma-server 2>/dev/null || true`,
		`tmux kill-session -t s3-rdma-server 2>/dev/null || true`,
		`tmux new-session -d -s s3-rdma-server`,
	}

	deploymentCmd := "cd ~/rdma-demo && sudo ./s3-rdma-server"

	deploymentCmd += " --payload-root=assets --enable-rdma-zcopy=true"

	log.Infof("Deploying RDMA storage on worker nodes: %v", workerNodeSetup.StorageNodes)
	var wg sync.WaitGroup
	for _, storageNode := range workerNodeSetup.StorageNodes {
		go func(node string) {
			defer wg.Done()
			deploymentCmd += " --tcp-listen=" + node + ":10090"
			deploymentCmd += " --rdma-zcopy-listen=" + node + ":10191"
			CommandList = append(CommandList, fmt.Sprintf(`tmux send-keys -t s3-rdma-server "%s" C-m`, deploymentCmd))
			for _, cmd := range CommandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on storage node %s: %v", cmd, node, err)
				}
			}
			log.Infof("RDMA storage deployed on storage node %s", node)
		}(storageNode)
		wg.Add(1)
	}
	wg.Wait()
}

func CleanKhala(workerNodeSetup WorkerNodeSetup, removeSnapshots bool) {
	log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
	CommandList := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`sudo rm -rf ~/khala/runtime/overlayfs/*.overlay`,
		`sudo rm -rf ~/khala/runtime/logs/*.log`,
		`sudo rm -rf ~/khala/runtime/metrics/*.metrics`,
		`sudo rm -rf ~/khala/runtime/uffd_sock/*.sock`,
		`bash -c 'cd ~/khala && bash cleanup_worker.sh'`,
	}
	if removeSnapshots {
		CommandList = append(CommandList,
			`sudo rm -rf ~/khala/runtime/snapshots/*.snapshot`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.mem`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.trace`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.ws`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.overlay`,
		)
	}

	var wg sync.WaitGroup
	var khalaDied atomic.Bool
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		go func(node string) {
			defer wg.Done()
			conn, err := grpc.NewClient(node+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Errorf("Failed to connect to nexus endpoint %s: %v", node, err)
				khalaDied.Store(true)
			}
			defer conn.Close()
			client := proto.NewKhalaKnativeIntegrationClient(conn)

			destroy_all_ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			_, err = client.DestroyAll(destroy_all_ctx, &proto.DestroyAllRequest{DestroyAll: true})
			if err != nil {
				log.Errorf("Failed to destroy all on nexus endpoint %s: %v", node, err)
				khalaDied.Store(true)
			}

			for _, cmd := range CommandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on nexus endpoint %s: %v", cmd, node, err)
				}
			}

		}(workerNode)
		wg.Add(1)
	}
	wg.Wait()

	out, err := loaderUtils.ServerExec("10.0.1.1", "bash -c 'cd ~/loader && bash cleanup_etcd.sh'")
	if err != nil {
		log.Errorf("Failed to clean etcd: %v, output: %s", err, out)
	}

	// 3. cleanup loader
	cmd := exec.Command("bash", "-c", "cd ~/loader && make clean && sleep 1 && make clean")
	out_byte, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clean loader: %v, output: %s", err, string(out_byte))
	}

	// 4. restart cluster components
	if khalaDied.Load() {
		CleanupCmd := []string{
			"kubectl rollout restart daemonset calico-node -n kube-system",
			"kubectl rollout status daemonset calico-node -n kube-system",
			"sleep 10",
			"kubectl rollout restart deployment calico-kube-controllers -n kube-system",
			"kubectl rollout status deployment calico-kube-controllers -n kube-system",
			"sleep 10",
		}
		log.Infof("Khala appears to have died on one or more worker nodes, restarting calico")
		for _, cmd := range CleanupCmd {
			cmd := exec.Command("bash", "-c", cmd)
			out_byte, err := cmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(out_byte))
			}
		}
	}
	{
		CleanupCmd := []string{
			"kubectl rollout restart -n knative-serving deployment/activator",
			"kubectl rollout status -n knative-serving deployment/activator",
			"sleep 10",
		}
		log.Infof("Restarting knative activator")
		for _, cmd := range CleanupCmd {
			cmd := exec.Command("bash", "-c", cmd)
			out_byte, err := cmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(out_byte))
			}
		}
	}

	log.Infof("Cleaning up minio")
	if khalaDied.Load() {
		out, err = loaderUtils.ServerExec("10.0.1.1", "bash -c 'source /etc/profile && cd ~/loader/scripts/setup && go run setup.go --setup-type=cleanup_minio --config=node_setup.json'")
		if err != nil {
			log.Errorf("Failed to clean minio: %v, output: %s", err, out)
		}
		time.Sleep(10 * time.Second)
		out, err = loaderUtils.ServerExec("10.0.1.1", "bash -c 'source /etc/profile && cd ~/loader/scripts/setup && go run setup.go --setup-type=redeploy_minio --config=node_setup.json'")
		if err != nil {
			log.Errorf("Failed to redeploy minio: %v, output: %s", err, out)
		}

		time.Sleep(60 * time.Second)
	}

	cmd = exec.Command("bash", "-c", "cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	out_byte, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to cleanup minio: %v, output: %s", err, string(out_byte))
	}

	log.Infof("Khala cleaned on all worker nodes")
}

func CreateSnapshots(workerNodeSetup WorkerNodeSetup) {
	// workloadList := []string{"streducer-s3-rpc-0", "streducer-0"}
	var workloadList []string

	for _, workload := range WorkloadList {
		if *SetNexusSDK {
			workload += "-s3"
		}
		if *SetNexusRPC {
			workload += "-rpc"
		}
		workload += "-0"
		workloadList = append(workloadList, workload)
	}

	var wg sync.WaitGroup
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		go func(node string) {
			defer wg.Done()
			conn, err := grpc.NewClient(node+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Errorf("Failed to connect to nexus endpoint %s: %v", node, err)
			}
			defer conn.Close()
			client := proto.NewKhalaKnativeIntegrationClient(conn)

			for _, workload := range workloadList {
				_, err = client.CreateSnapshot(context.Background(), &proto.CreateSnapshotRequest{Workload: workload})
				if err != nil {
					log.Errorf("Failed to create snapshot for function %s on nexus endpoint %s: %v", workload, node, err)
				} else {
					log.Infof("Snapshot created for function %s on nexus endpoint %s", workload, node)
				}
			}

		}(workerNode)
		wg.Add(1)
	}
	wg.Wait()
}

func SetManualCorePool(node string, corePoolSetting string, workerNodeSetup WorkerNodeSetup) {
	// parse core pool setting
	// 'C:18@2.1,IO:10@1.0'
	// means set core pool for CPU-intensive functions to 18 with frequency scaling factor 2.1
	// and for IO-intensive functions to 10 with frequency scaling factor 1.0

	//parse corePoolSize
	conn, err := grpc.NewClient(node+":8002", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Errorf("Failed to connect to hardware manager on node %s: %v", node, err)
		return
	}
	defer conn.Close()
	client := proto.NewHardwareManagerClient(conn)

	// set core pool
	corePoolList := corePoolParser(corePoolSetting)
	for _, corePool := range corePoolList {
		_, err = client.SetCorePool(context.Background(), &corePool)
		if err != nil {
			log.Errorf("Failed to set core pool on node %s: %v", node, err)
		} else {
			log.Infof("Set core pool %v on node %s", corePool, node)
		}
	}
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
	for _, corePool := range corePoolList {
		_, err = client.SetCorePool(context.Background(), &corePool)
		if err != nil {
			log.Errorf("Failed to set core pool on node %s: %v", node, err)
			return err
		} else {
			log.Infof("Set core pool %v on node %s", corePool, node)
		}
	}

	return nil
}
