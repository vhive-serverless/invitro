package cluster

import (
	"fmt"
	"time"

	"github.com/vhive-serverless/loader/scripts/setup/configs"
	loaderUtils "github.com/vhive-serverless/loader/scripts/setup/utils"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

// setupPrometheus sets up Prometheus components on the master node.
func setupPrometheus(masterNode string, allNode []string, promConfig *configs.PrometheusConfig) error {
	// Install htop
	utils.WaitPrintf("Installing htop on master node %s...\n", masterNode)
	_, err := loaderUtils.ServerExec(masterNode, "sudo apt install htop")
	if !utils.CheckErrorWithMsg(err, "Failed to install htop on master node %s: %v\n", masterNode, err) {
		return err
	}

	for _, node := range allNode {
		utils.WaitPrintf("Set Paranoid level on node %s...\n", node)
		_, err := loaderUtils.ServerExec(node, "sudo sysctl -w kernel.perf_event_paranoid=-1")
		if !utils.CheckErrorWithMsg(err, "Failed to set Paranoid level on node %s: %v\n", node, err) {
			return err
		}
	}

	// Deploy Metrics Server
	utils.WaitPrintf("Deploying Metrics Server version %s on master node %s...\n", promConfig.MetricsServerVersion, masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/%s/components.yaml", promConfig.MetricsServerVersion))
	if !utils.CheckErrorWithMsg(err, "Failed to deploy Metrics Server on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Patch Metrics Server deployment to allow insecure TLS
	utils.WaitPrintf("Patching Metrics Server deployment for insecure TLS on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, `kubectl patch deployment metrics-server -n kube-system --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls=true"}]'`)
	if !utils.CheckErrorWithMsg(err, "Failed to patch Metrics Server deployment on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Install Helm
	utils.WaitPrintf("Installing Helm on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash")
	if !utils.CheckErrorWithMsg(err, "Failed to install Helm on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Add Prometheus Helm repository
	utils.WaitPrintf("Adding Prometheus Helm repository on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm repo add prometheus-community https://prometheus-community.github.io/helm-charts")
	if !utils.CheckErrorWithMsg(err, "Failed to add Prometheus Helm repository on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Update Helm repositories
	utils.WaitPrintf("Updating Helm repositories on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "helm repo update")
	if !utils.CheckErrorWithMsg(err, "Failed to update Helm repositories on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Create monitoring namespace
	utils.WaitPrintf("Creating 'monitoring' namespace on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "kubectl create namespace monitoring")
	if !utils.CheckErrorWithMsg(err, "Failed to create 'monitoring' namespace on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Install and start Prometheus stack using helm
	utils.WaitPrintf("Installing Prometheus stack (version %s) on master node %s...\n", promConfig.PromChartVersion, masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("helm install -n monitoring prometheus --version %s prometheus-community/kube-prometheus-stack -f %s/prom_values.yaml", promConfig.PromChartVersion, promConfig.PromValuePath))
	if !utils.CheckErrorWithMsg(err, "Failed to install Prometheus stack on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Apply ServiceMonitors/PodMonitors for Knative metrics
	utils.WaitPrintf("Applying Knative ServiceMonitors/PodMonitors on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("curl -sL %s/config/serving-monitors.yaml | sed 's/interval: 30s/interval: 2s/g' | kubectl apply -f -", promConfig.KnativePromURL))
	if !utils.CheckErrorWithMsg(err, "Failed to apply Knative ServiceMonitors/PodMonitors on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Apply Grafana dashboards for Knative
	utils.WaitPrintf("Applying Knative Grafana dashboards on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf(`curl -sL %s/config/configmap-serving-dashboard.json | sed 's/"namespace": "knative-serving"/"namespace": "monitoring"/g' | kubectl apply -f -`, promConfig.KnativePromURL))
	if !utils.CheckErrorWithMsg(err, "Failed to apply Knative Grafana dashboards on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Bind addresses of the control manager and scheduler to "0.0.0.0"
	utils.WaitPrintf("Binding control manager and scheduler addresses on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, fmt.Sprintf("sudo kubeadm upgrade apply --config %s/kubeadm_init.yaml --ignore-preflight-errors all --force --v=7", promConfig.PromValuePath))
	if !utils.CheckErrorWithMsg(err, "Failed to bind control manager and scheduler addresses on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Restart the kube-proxy
	utils.WaitPrintf("Restarting kube-proxy on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "kubectl delete pod -l k8s-app=kube-proxy -n kube-system")
	if !utils.CheckErrorWithMsg(err, "Failed to restart kube-proxy on master node %s: %v\n", masterNode, err) {
		return err
	}

	time.Sleep(5 * time.Second)

	// Set up Prometheus port-forwarding in tmux
	utils.WaitPrintf("Setting up Prometheus port-forwarding on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "tmux new -s prometheusd -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session for Prometheus on master node %s: %v\n", masterNode, err) {
		return err
	}
	_, err = loaderUtils.ServerExec(masterNode, `tmux send -t prometheusd "while true; do kubectl port-forward -n monitoring service/prometheus-kube-prometheus-prometheus 9090; done" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to set up Prometheus port-forwarding in tmux on master node %s: %v\n", masterNode, err) {
		return err
	}

	// Set up Grafana dashboard port-forwarding in tmux (id: admin, pwd: prom-operator)
	utils.WaitPrintf("Setting up Grafana dashboard port-forwarding on master node %s...\n", masterNode)
	_, err = loaderUtils.ServerExec(masterNode, "tmux new -s grafanad -d")
	if !utils.CheckErrorWithMsg(err, "Failed to create tmux session for Grafana on master node %s: %v\n", masterNode, err) {
		return err
	}
	_, err = loaderUtils.ServerExec(masterNode, `tmux send -t grafanad "while true; do kubectl -n monitoring port-forward deployment/prometheus-grafana 3000; done" ENTER`)
	if !utils.CheckErrorWithMsg(err, "Failed to set up Grafana dashboard port-forwarding in tmux on master node %s: %v\n", masterNode, err) {
		return err
	}

	utils.InfoPrintf("Done setting up Prometheus components.\n")
	return nil
}
