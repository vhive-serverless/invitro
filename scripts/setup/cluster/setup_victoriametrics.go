package cluster

import (
	"time"

	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

// setupVictoriaMetrics sets up VictoriaMetrics components on the master node.
func setupVictoriaMetrics(masterNode string, monitorNode string) error {
	// Install htop
	utils.WaitPrintf("Installing htop on master node %s...\n", masterNode)
	_, err := loaderUtils.ServerExec(masterNode, "sudo apt install htop")
	if !utils.CheckErrorWithMsg(err, "Failed to install htop on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Install Helm
	// check if helm is already installed
	utils.WaitPrintf("Installing Helm on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm version --short")
	if err != nil {
		_, err = loaderUtils.ServerExec(masterNode, "curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash")
		if !utils.CheckErrorWithMsg(err, "Failed to install Helm on master node %s: %v\n", masterNode, err) {
			return err
		}
	}

	time.Sleep(1 * time.Second)

	// Add VictoriaMetrics Helm repository
	utils.WaitPrintf("Adding VictoriaMetrics Helm repository on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm repo add vm https://victoriametrics.github.io/helm-charts")
	if !utils.CheckErrorWithMsg(err, "Failed to add VictoriaMetrics Helm repository on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Update Helm repositories
	utils.WaitPrintf("Updating Helm repositories on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm repo update")
	if !utils.CheckErrorWithMsg(err, "Failed to update Helm repositories on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Create VictoriaMetrics Local Storage directory on Monitoring Node
	utils.WaitPrintf("Creating VictoriaMetrics Local Storage directory on monitoring node %s...\n", monitorNode)
	_, err = loaderUtils.ServerExec(monitorNode, "sudo mkdir -p /mnt/data/victoria-metrics && sudo mkdir -p /mnt/resources/victoria-metrics")
	if !utils.CheckErrorWithMsg(err, "Failed to create VictoriaMetrics Local Storage directory on monitoring node %s: %v\n", monitorNode, err) {
		return err
	}

	utils.WaitPrintf("Creating VictoriaMetrics Local Storage mount on monitoring node %s...\n", monitorNode)
	_, err = loaderUtils.ServerExec(monitorNode, "sudo mount --bind /mnt/data/victoria-metrics /mnt/resources/victoria-metrics")
	if !utils.CheckErrorWithMsg(err, "Failed to mount VictoriaMetrics Local Storage on monitoring node %s: %v\n", monitorNode, err) {
		return err
	}

	// Apply PV manifest for VictoriaMetrics
	utils.WaitPrintf("Applying PV manifest for VictoriaMetrics on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "kubectl apply -f ~/loader/scripts/setup/configs/victoriametrics/vm-pv.yaml")
	if !utils.CheckErrorWithMsg(err, "Failed to apply PV manifest for VictoriaMetrics on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Install VictoriaMetrics
	utils.WaitPrintf("Installing VictoriaMetrics on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm upgrade --install victoria-metrics vm/victoria-metrics-single --namespace monitoring --create-namespace -f ~/loader/scripts/setup/configs/victoriametrics/vm-values.yaml")
	if !utils.CheckErrorWithMsg(err, "Failed to install VictoriaMetrics on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Setup VictoriaMetrics port-forwarding in tmux
	utils.WaitPrintf("Setting up VictoriaMetrics port-forwarding in tmux on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "tmux new -s victoriametrics -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session for VictoriaMetrics on master node %s: %v\n", masterNode, err) {
		return err
	}
	_, err = loaderUtils.ServerExec(masterNode, `tmux send -t victoriametrics "while true; do kubectl -n monitoring port-forward deployment/victoria-metrics-single 8428:8428; done" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to set up VictoriaMetrics port-forwarding in tmux on master node %s: %v\n", masterNode, err) {
		return err
	}

	utils.InfoPrintf("Done setting up VictoriaMetrics components.\n")
	return nil
}
