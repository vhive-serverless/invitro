package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sync"
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
	Debug           = flag.Bool("debug", false, "Enable debug mode")
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
		DeployKhala(workerNodeSetup, *CorePoolPolicy, *Implementation, *Debug)
	case "clean":
		log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
		CleanKhala(workerNodeSetup, *RemoveSnapshots)
	default:
		log.Fatalf("Unknown command: %s", *Command)
	}

}

type WorkerNodeSetup struct {
	WorkerNodes []string `json:"worker_nodes"`
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
	return workerNodeSetup, nil
}

func DeployKhala(workerNodeSetup WorkerNodeSetup, corePoolPolicy string, implementation string, debug bool) error {
	// 1. cleanup minio
	log.Infof("Cleaning up minio")
	cmd := exec.Command("bash", "-c", "cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalf("Failed to cleanup minio: %v, output: %s", err, string(output))
		return err
	}

	deploymentCmd := "cd ~/khala && sudo ./bin/kn-integration"
	deploymentCmd += " --impl=" + implementation
	if corePoolPolicy != "" {
		deploymentCmd += " --corepool=" + corePoolPolicy
	}

	if debug {
		deploymentCmd += " > ~/khala/kn-integration.log 2>&1"
	}

	CommandList := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`tmux new-session -d -s kn-integration`,
		fmt.Sprintf(`tmux send-keys -t kn-integration "%s" C-m`, deploymentCmd),
	}

	// 2. deploy khala on all worker nodes
	log.Infof("Deploying Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
	var wg sync.WaitGroup
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		go func(node string) {
			defer wg.Done()
			for _, cmd := range CommandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on worker node %s: %v", cmd, workerNode, err)
				}
			}
			log.Infof("Khala deployed on worker node %s", workerNode)
		}(workerNode)
		wg.Add(1)
	}
	wg.Wait()
	time.Sleep(30 * time.Second)

	return nil
}

func CleanKhala(workerNodeSetup WorkerNodeSetup, removeSnapshots bool) {
	log.Infof("Cleaning Khala on worker nodes: %v", workerNodeSetup.WorkerNodes)
	CommandList := []string{
		`sudo pkill --signal INT kn-integration 2>/dev/null || true`,
		`tmux kill-session -t kn-integration 2>/dev/null || true`,
		`sudo rm -rf ~/khala/runtime/overlayfs/*.overlay`,
		`sudo rm -rf ~/khala/runtime/logs/*.log`,
		`bash -c 'cd ~/khala && bash cleanup_worker.sh'`,
	}
	if removeSnapshots {
		CommandList = append(CommandList,
			`sudo rm -rf ~/khala/runtime/snapshots/*.snapshot`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.mem`,
			`sudo rm -rf ~/khala/runtime/snapshots/*.overlay`,
		)
	}

	var wg sync.WaitGroup
	var khalaDied bool
	for _, workerNode := range workerNodeSetup.WorkerNodes {
		go func(node string, khalaDied *bool) {
			defer wg.Done()
			conn, err := grpc.NewClient(node+":8000", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Errorf("Failed to connect to nexus endpoint %s: %v", node, err)
				*khalaDied = true
			}
			defer conn.Close()
			client := proto.NewKhalaKnativeIntegrationClient(conn)

			_, err = client.DestroyAll(context.Background(), &proto.DestroyAllRequest{DestroyAll: true})
			if err != nil {
				log.Errorf("Failed to destroy all on nexus endpoint %s: %v", node, err)
				*khalaDied = true
			}
			*khalaDied = false

			for _, cmd := range CommandList {
				_, err := loaderUtils.ServerExec(node, cmd)
				if err != nil {
					log.Errorf("Failed to execute command '%s' on nexus endpoint %s: %v", cmd, node, err)
				}
			}

		}(workerNode, &khalaDied)
		wg.Add(1)
	}
	wg.Wait()

	loaderUtils.ServerExec("10.0.1.1", "bash -c 'cd ~/khala && bash cleanup_etcd.sh'")

	// 3. cleanup loader
	cmd := exec.Command("bash", "-c", "cd ~/loader && make clean && sleep 1 && make clean")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to clean loader: %v, output: %s", err, string(output))
	}

	// 4. restart cluster components
	if khalaDied {
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
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(output))
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
			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Errorf("Failed to execute command '%s': %v, output: %s", cmd, err, string(output))
			}
		}
	}

	log.Infof("Cleaning up minio")
	cmd = exec.Command("bash", "-c", "cd ~/khala && bash ./scripts/deploy-minio-obj.sh http://myminio-api.minio.10.200.3.4.sslip.io")
	output, err = cmd.CombinedOutput()
	if err != nil {
		log.Errorf("Failed to cleanup minio: %v, output: %s", err, string(output))
	}
	log.Infof("Khala cleaned on all worker nodes")
}
