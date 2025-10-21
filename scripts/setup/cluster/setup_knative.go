package cluster

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func commonInit(nodes []string, cfg *configs.SetupConfig, operationMode string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(nodes))

	for _, node := range nodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.InfoPrintf("Initializing node: %s\n", node)
			// Clone vHive repository
			utils.WaitPrintf("Cloning vHive repository on node %s...\n", node)
			_, err := loaderUtils.ServerExec(node, fmt.Sprintf("git clone --branch=%s %s", cfg.HiveBranch, cfg.HiveRepo))
			if !utils.CheckErrorWithMsg(err, "Failed to clone vHive repository on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Install Go and setup node
			utils.WaitPrintf("Installing Go and setting up node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, fmt.Sprintf("pushd ~/vhive/scripts > /dev/null && ./install_go.sh && source /etc/profile && go build -o setup_tool && ./setup_tool setup_node %s && popd > /dev/null", operationMode))
			if !utils.CheckErrorWithMsg(err, "Failed to install Go and setup node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Start containerd in tmux
			utils.WaitPrintf("Starting containerd in tmux on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, "tmux new -s containerd -d")
			if !utils.CheckErrorWithMsg(err, "Failed to create tmux session for containerd on node %s: %v \n", node, err) {
				errChan <- err
				return
			}
			_, err = loaderUtils.ServerExec(node, `tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER`)
			if !utils.CheckErrorWithMsg(err, "Failed to start containerd in tmux on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Install chrony, htop, sysstat
			utils.WaitPrintf("Installing chrony, htop, sysstat on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, "sudo apt-get update && sudo apt-get install -y chrony htop sysstat etcd-client")
			if !utils.CheckErrorWithMsg(err, "Failed to install chrony, htop, sysstat on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Configure chrony
			utils.WaitPrintf("Configuring chrony on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, `echo "server ops.emulab.net iburst prefer" | sudo tee -a /etc/chrony/chrony.conf >/dev/null`)
			if !utils.CheckErrorWithMsg(err, "Failed to configure chrony on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Restart chronyd
			utils.WaitPrintf("Restarting chronyd on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, "sudo systemctl restart chronyd")
			if !utils.CheckErrorWithMsg(err, "Failed to restart chronyd on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Check chrony tracking
			utils.WaitPrintf("Checking chrony tracking on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, "sudo chronyc tracking")
			if !utils.CheckErrorWithMsg(err, "Failed to check chrony tracking on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Clone loader repository
			utils.WaitPrintf("Cloning loader repository on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, fmt.Sprintf("git clone --depth=1 --branch=%s %s loader", cfg.LoaderBranch, cfg.LoaderRepo))
			if !utils.CheckErrorWithMsg(err, "Failed to clone loader repository on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Install python3-pip
			utils.WaitPrintf("Installing python3-pip on node %s...\n", node)
			_, err = loaderUtils.ServerExec(node, `echo -en "\n\n" | sudo apt-get install -y python3-pip`)
			if !utils.CheckErrorWithMsg(err, "Failed to install python3-pip on node %s: %v \n", node, err) {
				errChan <- err
				return
			}

			// Run stabilize script
			// utils.WaitPrintf("Running stabilize script on node %s...\n", node)
			// _, err = loaderUtils.ServerExec(node, "~/loader/scripts/setup/stabilize.sh")
			// if !utils.CheckErrorWithMsg(err, "Failed to run stabilize script on node %s: %v \n", node, err) {
			// 	errChan <- err
			// 	return
			// }
		}(node)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}
	return nil
}

func setupMaster(masterNode string, operationMode string) (string, error) {
	setupCmd := fmt.Sprintf("pushd ~/vhive/scripts > /dev/null && ./setup_tool create_multinode_cluster %s && popd > /dev/null", operationMode)
	// Create tmux sessions
	_, err := loaderUtils.ServerExec(masterNode, "tmux new -s runner -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session 'runner' on master node %s: %v \n", masterNode, err) {
		return "", err
	}
	_, err = loaderUtils.ServerExec(masterNode, "tmux new -s master -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session 'master' on master node %s: %v\n", masterNode, err) {
		return "", err
	}

	// Rewrite YAML files
	_, err = loaderUtils.ServerExec(masterNode, "~/loader/scripts/setup/rewrite_yaml_files.sh")
	if !utils.CheckErrorWithMsg(err, "Failed to rewrite YAML files on master node %s: %v\n", masterNode, err) {
		return "", err
	}

	// Execute setup command in master tmux session
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf(`tmux send -t master "%s" ENTER`, setupCmd))
	if !utils.CheckErrorWithMsg(err, "Failed to send setup command to master tmux session on master node %s: %v\n", masterNode, err) {
		return "", err
	}

	// Wait for masterKey.yaml to be created
	utils.InfoPrintf("Waiting for master key to be generated...\n")
	for {
		_, err := loaderUtils.ServerExec(masterNode, "stat ~/vhive/scripts/masterKey.yaml")
		if err == nil {
			break
		}
		time.Sleep(2 * time.Second)
	}
	utils.InfoPrintf("Master key found.\n")

	// Get the join token
	getTokenCmd := `awk '/^ApiserverAdvertiseAddress:/ {ip=$2} /^ApiserverPort:/ {port=$2} /^ApiserverToken:/ {token=$2} /^ApiserverTokenHash:/ {token_hash=$2} END {print "sudo kubeadm join " ip ":" port " --token " token " --discovery-token-ca-cert-hash " token_hash}' ~/vhive/scripts/masterKey.yaml`
	joinToken, err := loaderUtils.ServerExec(masterNode, getTokenCmd)
	if !utils.CheckErrorWithMsg(err, "Failed to get join token from master: %v\n", err) {
		return "", err
	}

	return strings.TrimSpace(joinToken), nil
}

func setupWorkers(workerNodes []string, joinToken string, cfg *configs.SetupConfig, operationMode string) error {
	var wg sync.WaitGroup
	errChan := make(chan error, len(workerNodes))

	for _, node := range workerNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.InfoPrintf("Setting up worker: %s\n", node)

			// Setup worker kubelet
			utils.WaitPrintf("Setting up worker kubelet on node %s...\n", node)
			setupKubeletCmd := fmt.Sprintf("pushd ~/vhive/scripts > /dev/null && ./setup_tool setup_worker_kubelet %s && popd > /dev/null", operationMode)
			_, err := loaderUtils.ServerExec(node, setupKubeletCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to setup worker kubelet on worker %s: %v\n", node, err) {
				errChan <- err
				return
			}

			// Setup vHive Firecracker daemon if operation mode is firecracker
			if operationMode == "firecracker" {
				err = setupVhiveFirecrackerDaemon(node, cfg.ClusterMode)
				if !utils.CheckErrorWithMsg(err, "Failed to setup vHive Firecracker daemon on worker %s: %v\n", node, err) {
					errChan <- err
					return
				}
			}

			// Join token
			_, err = loaderUtils.ServerExec(node, joinToken)
			if !utils.CheckErrorWithMsg(err, "Failed to join worker %s with token: %v\n", node, err) {
				errChan <- err
				return
			}

			// Configure maxPods
			_, err = loaderUtils.ServerExec(node, fmt.Sprintf("echo \"maxPods: %d\" | sudo tee -a /var/lib/kubelet/config.yaml >/dev/null", cfg.PodsPerNode))
			if !utils.CheckErrorWithMsg(err, "Failed to set maxPods on worker %s: %v\n", node, err) {
				errChan <- err
				return
			}

			// Configure containerLogMaxSize
			_, err = loaderUtils.ServerExec(node, `echo "containerLogMaxSize: 512Mi" | sudo tee -a /var/lib/kubelet/config.yaml >/dev/null`)
			if !utils.CheckErrorWithMsg(err, "Failed to set containerLogMaxSize on worker %s: %v\n", node, err) {
				errChan <- err
				return
			}

			// Restart kubelet
			utils.WaitPrintf("Restarting kubelet on worker %s...\n", node)
			_, err = loaderUtils.ServerExec(node, "sudo systemctl restart kubelet")
			if !utils.CheckErrorWithMsg(err, "Failed to restart kubelet on worker %s: %v\n", node, err) {
				errChan <- err
				return
			}

			time.Sleep(10 * time.Second)

			// Rejoin after restart
			utils.WaitPrintf("Rejoining worker %s after kubelet restart...\n", node)
			_, _ = loaderUtils.ServerExec(node, joinToken)
			utils.InfoPrintf("Worker node %s has joined the cluster.\n", node)
		}(node)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		return err
	}
	return nil
}

func extendCIDR(masterNode string, workerNodes []string, joinToken string) error {
	utils.InfoPrintf("Extending CIDR for all nodes...\n")

	// Get node names first
	nodeList, err := loaderUtils.ServerExec(masterNode, "kubectl get no | tail -n +2 | awk '{print $1}'")
	if !utils.CheckErrorWithMsg(err, "Failed to get node names from master node %s: %v\n", masterNode, err) {
		return err
	}
	nodeNames := strings.Fields(nodeList)
	if len(nodeNames) == 0 {
		return fmt.Errorf("could not retrieve any node names")
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(nodeNames))

	for i, nodeName := range nodeNames {
		wg.Add(1)
		go func(nodeName string, i int) {
			defer wg.Done()
			subnet := i*4 + 4
			// Get node JSON, modify podCIDR, and save to node.yaml
			getNodeCmd := fmt.Sprintf("kubectl get node %s -o json | jq '.spec.podCIDR |= \"10.168.%d.0/22\"' > node.yaml", nodeName, subnet)
			_, err := loaderUtils.ServerExec(masterNode, getNodeCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to get and modify node %s JSON on master node %s: %v\n", nodeName, masterNode, err) {
				errChan <- err
				return
			}

			// Delete the node
			deleteNodeCmd := fmt.Sprintf("kubectl delete node %s", nodeName)
			_, err = loaderUtils.ServerExec(masterNode, deleteNodeCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to delete node %s on master node %s: %v\n", nodeName, masterNode, err) {
				errChan <- err
				return
			}

			// Create the node with updated CIDR
			createNodeCmd := "kubectl create -f node.yaml"
			_, err = loaderUtils.ServerExec(masterNode, createNodeCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to create node %s with updated CIDR on master node %s: %v\n", nodeName, masterNode, err) {
				errChan <- err
				return
			}
			utils.InfoPrintf("Changed pod CIDR for node %s to 10.168.%d.0/22\n", nodeName, subnet)
		}(nodeName, i)
	}

	wg.Wait()
	close(errChan)
	for err := range errChan {
		return err
	}

	// Rejoin the cluster for the 3rd time
	for _, node := range workerNodes {
		_, _ = loaderUtils.ServerExec(node, joinToken)
	}

	return nil
}

func finalizeClusterSetup(masterNode string, allNodes []string) error {
	// Untaint master node
	untaintCmd := "kubectl taint nodes $(hostname) node-role.kubernetes.io/control-plane-"
	var err error // Declare err once
	_, err = loaderUtils.ServerExec(masterNode, untaintCmd)
	if !utils.CheckErrorWithMsg(err, "Failed to untaint master node %s: %v\n", masterNode, err) {
		return err
	}

	// Notify the master that all nodes have joined
	_, err = loaderUtils.ServerExec(masterNode, `tmux send -t master "y" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to notify master node %s: %v\n", masterNode, err) {
		return err
	}

	// Wait for knative-serving namespace
	utils.InfoPrintf("Waiting for knative-serving namespace to be created...\n")
	for {
		out, innerErr := loaderUtils.ServerExec(masterNode, "kubectl get namespaces") // Use innerErr for loop scope
		if innerErr == nil && strings.Contains(out, "knative-serving") {
			break
		}
		time.Sleep(15 * time.Second)
	}
	utils.InfoPrintf("Knative-serving namespace is ready.\n")

	// Copy kubeconfig from master to all other nodes
	err = copyK8sCertificates(masterNode, allNodes)
	if !utils.CheckErrorWithMsg(err, "Failed to copy kubeconfig: %v\n", err) {
		return err
	}

	// Patch init scale
	_, err = loaderUtils.ServerExec(masterNode, "cd ~/loader; bash scripts/setup/patch_init_scale.sh")
	if !utils.CheckErrorWithMsg(err, "Failed to run patch_init_scale.sh on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Enable affinity
	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch configmap -n knative-serving config-features -p '{"data": {"kubernetes.podspec-affinity": "enabled"}}'`)
	if !utils.CheckErrorWithMsg(err, "Failed to enable affinity on master node %s: %v\n", masterNode, err) {
		return err
	}

	return nil
}

func copyK8sCertificates(masterNode string, allNodes []string) error {
	utils.InfoPrintf("Copying K8s certificates from master to all nodes...\n")
	kubeconfig, err := loaderUtils.ServerExec(masterNode, "cat ~/.kube/config")
	if !utils.CheckErrorWithMsg(err, "Failed to get kubeconfig from master node %s: %v\n", masterNode, err) {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(allNodes)-1)
	for _, node := range allNodes {
		if node == masterNode {
			continue
		}
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			// Create .kube directory
			mkdirCmd := "mkdir -p ~/.kube"
			_, err := loaderUtils.ServerExec(node, mkdirCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to create .kube directory on node %s: %v\n", node, err) {
				errChan <- err
				return
			}

			// Write kubeconfig to file
			echoKubeconfigCmd := fmt.Sprintf("echo '%s' > ~/.kube/config", kubeconfig)
			_, err = loaderUtils.ServerExec(node, echoKubeconfigCmd)
			if !utils.CheckErrorWithMsg(err, "Failed to write kubeconfig to node %s: %v\n", node, err) {
				errChan <- err
			}
		}(node)
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		return err
	}
	utils.InfoPrintf("Successfully copied K8s certificates.\n")
	return nil
}

func distributeLoaderSSHKey(masterNode string, allNodes []string) error {
	_, err := loaderUtils.ServerExec(masterNode, `echo -e "\n\n\n" | ssh-keygen -t rsa -N ""`)
	if !utils.CheckErrorWithMsg(err, "Failed to generate SSH key on loader node %s: %v\n", masterNode, err) {
		return err
	}

	pubKey, err := loaderUtils.ServerExec(masterNode, "cat ~/.ssh/id_rsa.pub")
	if !utils.CheckErrorWithMsg(err, "Failed to get public key from loader node %s: %v\n", masterNode, err) {
		return err
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(allNodes))
	for _, node := range allNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			cmd := fmt.Sprintf("echo '%s' >> ~/.ssh/authorized_keys", strings.TrimSpace(pubKey))
			_, err := loaderUtils.ServerExec(node, cmd)
			if !utils.CheckErrorWithMsg(err, "Failed to distribute SSH key to node %s: %v\n", node, err) {
				errChan <- err
			}
		}(node)
	}
	wg.Wait()
	close(errChan)
	for err := range errChan {
		return err
	}
	utils.InfoPrintf("Successfully distributed loader SSH key.\n")
	return nil
}

// setupVhiveFirecrackerDaemon sets up the vHive Firecracker daemon on a given node.
func setupVhiveFirecrackerDaemon(node string, clusterMode string) error {
	// Build vHive
	_, err := loaderUtils.ServerExec(node, "cd vhive; source /etc/profile && go build")
	if !utils.CheckErrorWithMsg(err, "Failed to build vHive on node %s: %v\n", node, err) {
		return err
	}

	// Create tmux session for firecracker
	_, err = loaderUtils.ServerExec(node, "tmux new -s firecracker -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session 'firecracker' on node %s: %v\n", node, err) {
		return err
	}

	// Start firecracker-containerd
	_, err = loaderUtils.ServerExec(node, `tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee ~/firecracker_log.txt" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to start firecracker-containerd on node %s: %v\n", node, err) {
		return err
	}

	// Create tmux session for vhive
	_, err = loaderUtils.ServerExec(node, "tmux new -s vhive -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session 'vhive' on node %s: %v\n", node, err) {
		return err
	}

	// Change directory to vhive in tmux
	_, err = loaderUtils.ServerExec(node, `tmux send -t vhive "cd vhive" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to change directory to vhive in tmux on node %s: %v\n", node, err) {
		return err
	}

	// Determine firecracker snapshots argument based on cluster mode
	firecrackerSnapshots := ""
	if clusterMode == "firecracker_snapshots" {
		firecrackerSnapshots = "-snapshots"
	}

	// Run vhive
	runVhiveCmd := fmt.Sprintf("sudo ./vhive %s 2>&1 | tee ~/vhive_log.txt", firecrackerSnapshots)
	_, err = loaderUtils.ServerExec(node, fmt.Sprintf(`tmux send -t vhive "%s" ENTER`, runVhiveCmd))
	if !utils.CheckErrorWithMsg(err, "Failed to run vhive on node %s: %v\n", node, err) {
		return err
	}

	return nil
}
