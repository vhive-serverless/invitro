package cluster

import (
	"fmt"
	"sync"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func setupKhala(cfg *configs.SetupConfig, masterNode string, loaderNode string, workerNodes []string) error {
	var wg sync.WaitGroup
	var err error

	// clone local keys and gitconfig to master node
	// rsync -Pav ~/.gitconfig ${SERVER}:.
	// rsync -Pav ~/.ssh/id_ed25519* ${SERVER}:~/.ssh/
	utils.WaitPrintf("Cloning local SSH keys and gitconfig to master node: %s\n", masterNode)
	_, err = utils.ExecShellCmd("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh/id_ed25519* %s:~/.ssh/ && rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig %s:~/.gitconfig", masterNode, masterNode)
	if !utils.CheckErrorWithMsg(err, "Failed to clone SSH keys and gitconfig to node %s: %v\n", masterNode, err) {
		return err
	}

	// clone khala repo on master node
	utils.WaitPrintf("Cloning Khala repository on master node: %s\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("GIT_SSH_COMMAND='ssh -o StrictHostKeyChecking=no' git clone %s --branch %s", cfg.KhalaRepo, cfg.KhalaBranch))
	if !utils.CheckErrorWithMsg(err, "Failed to clone Khala repository on node %s: %v\n", masterNode, err) {
		return err
	}

	// get asset and build rootfs
	utils.WaitPrintf("Building Rootfs on master node: %s\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "cd khala && source /etc/profile && bash scripts/get_asset.sh &&  go run experiment-cmd/rebuild-squashfs/main.go -worker-ip 10.0.1.1")
	if !utils.CheckErrorWithMsg(err, "Failed to build Rootfs on node %s: %v\n", masterNode, err) {
		return err
	}

	// distribute keys from master node to all nodes (including loader and worker nodes)
	for _, node := range append([]string{loaderNode}, workerNodes...) {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.WaitPrintf("Distributing keys to node: %s\n", node)
			// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh "$i":~/ &
			_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh %s:~/", node))
			if !utils.CheckErrorWithMsg(err, "Failed to rsync SSH keys to node %s: %v\n", node, err) {
				return
			}

			// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig "$i":~/ &
			_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig %s:~/", node))
			if !utils.CheckErrorWithMsg(err, "Failed to rsync gitconfig to node %s: %v\n", node, err) {
				return
			}
		}(node)
	}

	wg.Wait()

	// distribute keys from master node to all nodes (including loader and worker nodes)
	for _, node := range append([]string{loaderNode}, workerNodes...) {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.WaitPrintf("Distributing Khala on node: %s\n", node)
			// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh "$i":~/ &
			_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/khala %s:~/", node))
			if !utils.CheckErrorWithMsg(err, "Failed to rsync Khala to node %s: %v\n", node, err) {
				return
			}
		}(node)
	}

	wg.Wait()

	for _, node := range append([]string{loaderNode}, workerNodes...) {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.WaitPrintf("Setting up Khala on node: %s\n", node)
			// cd khala && bash scripts/setup_knative.sh
			_, err := loaderUtils.ServerExec(node, "cd khala && bash scripts/setup_knative.sh && source /etc/profile && make build-all && sudo mkdir -p /mnt/resources/jailer")
			if !utils.CheckErrorWithMsg(err, "Failed to set up Khala on node %s: %v\n", node, err) {
				return
			}
		}(node)
	}

	wg.Wait()

	// ssh -oStrictHostKeyChecking=no "$i" "cd khala && bash scripts/setup_knative.sh"

	return nil
}
