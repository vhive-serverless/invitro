package cluster

import (
	"errors"
	"fmt"
	"sync"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func setupKhala(cfg *configs.SetupConfig, masterNode string, loaderNode string, workerNodes []string) error {
	var wg sync.WaitGroup
	var err error
	var setupErrors []error
	var errorsMu sync.Mutex
	addError := func(err error) {
		if err != nil {
			errorsMu.Lock()
			setupErrors = append(setupErrors, err)
			errorsMu.Unlock()
		}
	}

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

	// Fetch pinned assets and build the one Py/Go/JS evaluation rootfs.
	utils.WaitPrintf("Building Rootfs on master node: %s\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "cd khala && source /etc/profile && sudo env NEEDRESTART_SUSPEND=1 apt-get install -y git-lfs squashfs-tools xz-utils && bash scripts/get_asset.sh && bash scripts/build_evaluation_assets.sh")
	if !utils.CheckErrorWithMsg(err, "Failed to build Rootfs on node %s: %v\n", masterNode, err) {
		return err
	}

	// distribute keys from master node to all nodes (including loader and worker nodes)
	for _, node := range workerNodes {
		utils.WaitPrintf("Distributing keys to node: %s\n", node)
		// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh "$i":~/ &
		_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh %s:~/", node))
		if !utils.CheckErrorWithMsg(err, "Failed to rsync SSH keys to node %s: %v\n", node, err) {
			addError(fmt.Errorf("rsync SSH keys to %s: %w", node, err))
		}

		// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig "$i":~/ &
		_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.gitconfig %s:~/", node))
		if !utils.CheckErrorWithMsg(err, "Failed to rsync gitconfig to node %s: %v\n", node, err) {
			addError(fmt.Errorf("rsync gitconfig to %s: %w", node, err))
		}
	}

	wg.Wait()
	if err := errors.Join(setupErrors...); err != nil {
		return fmt.Errorf("distribute node credentials: %w", err)
	}

	// distribute keys from master node to all nodes (including loader and worker nodes)
	for _, node := range workerNodes {

		utils.WaitPrintf("Distributing Khala on node: %s\n", node)
		// rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/.ssh "$i":~/ &
		_, err := loaderUtils.ServerExec(masterNode, fmt.Sprintf("rsync -Pav -e 'ssh -o StrictHostKeyChecking=no' ~/khala %s:~/", node))
		if !utils.CheckErrorWithMsg(err, "Failed to rsync Khala to node %s: %v\n", node, err) {
			addError(fmt.Errorf("rsync Khala to %s: %w", node, err))
		}
	}

	wg.Wait()
	if err := errors.Join(setupErrors...); err != nil {
		return fmt.Errorf("distribute Khala: %w", err)
	}

	for _, node := range workerNodes {
		wg.Add(1)
		go func(node string) {
			defer wg.Done()
			utils.WaitPrintf("Setting up Khala on node: %s\n", node)
			_, err := loaderUtils.ServerExec(node, fmt.Sprintf("git clone --branch %s --single-branch %s firecracker", cfg.FirecrackerBranch, cfg.FirecrackerRepo))
			if !utils.CheckErrorWithMsg(err, "Failed to clone Firecracker source on node %s: %v\n", node, err) {
				addError(fmt.Errorf("clone Firecracker on %s: %w", node, err))
				return
			}
			// cd khala && bash scripts/setup_knative.sh
			_, err = loaderUtils.ServerExec(node, "cd khala && bash scripts/setup_knative.sh && source /etc/profile && make build-all build-nexus-backend-rdma && sudo mkdir -p /mnt/resources/jailer /mnt/resources/nexus-evaluation")
			if !utils.CheckErrorWithMsg(err, "Failed to set up Khala on node %s: %v\n", node, err) {
				addError(fmt.Errorf("set up Khala on %s: %w", node, err))
				return
			}
		}(node)
	}

	wg.Wait()

	// ssh -oStrictHostKeyChecking=no "$i" "cd khala && bash scripts/setup_knative.sh"

	return errors.Join(setupErrors...)
}
