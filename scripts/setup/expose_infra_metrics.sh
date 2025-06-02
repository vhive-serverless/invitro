#!/usr/bin/env bash
#
# MIT License
#
# Copyright (c) 2023 EASL and the vHive community
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.
#

MASTER_NODE=$1

server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $1 $2;
}

{
	echo 'Setting up monitoring components'
	server_exec $MASTER_NODE 'sudo apt install htop'

	#* Deploy Metrics Server to k8s in namespace kube-system.
	metrics_server_version="v0.7.2"
	server_exec $MASTER_NODE "kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/download/${metrics_server_version}/components.yaml"
	server_exec $MASTER_NODE "kubectl patch deployment metrics-server -n kube-system --type='json' -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls=true"}]'"

	#* Install helm.
	server_exec $MASTER_NODE 'curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash'
	#* Install and start prometheus stack using helm.
	server_exec $MASTER_NODE 'helm repo add prometheus-community https://prometheus-community.github.io/helm-charts'
	server_exec $MASTER_NODE 'helm repo update'

	server_exec $MASTER_NODE 'kubectl create namespace monitoring'

	for node in "$@"
	do
		server_exec $node 'sudo sysctl -w kernel.perf_event_paranoid=-1'
	done

	release_label="prometheus"
	prometheus_chart_version="72.6.2"
	server_exec $MASTER_NODE "cd loader; helm install -n monitoring $release_label --version $prometheus_chart_version prometheus-community/kube-prometheus-stack -f config/prometh_values_kn.yaml"

	#* Apply the ServiceMonitors/PodMonitors to collect metrics from Knative.
	#* The ports of the control manager and scheduler are mapped in a way that prometheus default installation can find them. 
	#* Also apply the grafana dashboards for Knative.
	server_exec $MASTER_NODE "curl -s https://raw.githubusercontent.com/knative-extensions/monitoring/main/servicemonitor.yaml | sed 's/interval: 30s/interval: 2s/g' | kubectl apply -f -"
	server_exec $MASTER_NODE 'kubectl apply -f https://raw.githubusercontent.com/knative-extensions/monitoring/main/grafana/dashboards.yaml'

	#* Bind addresses of the control manager and scheduler to "0.0.0.0" so that prometheus can scrape them from any domains.
	server_exec $MASTER_NODE 'cd loader; sudo kubeadm upgrade apply --config config/kubeadm_init.yaml --ignore-preflight-errors all --force --v=7'

	#* Restart the kube-proxy to apply the changes.
	server_exec $MASTER_NODE 'kubectl delete pod -l k8s-app=kube-proxy -n kube-system'

	sleep 5

	#* Set up port prometheus panel (infinite loops are important to circumvent kubectl timeout in the middle of experiments).
	server_exec $MASTER_NODE 'tmux new -s prometheusd -d'
	server_exec $MASTER_NODE 'tmux send -t prometheusd "while true; do kubectl port-forward -n monitoring service/prometheus-kube-prometheus-prometheus 9090; done" ENTER'

	#* Set up grafana dash board (id: admin, pwd: prom-operator).
	server_exec $MASTER_NODE 'tmux new -s grafanad -d'
	server_exec $MASTER_NODE 'tmux send -t grafanad "while true; do kubectl -n monitoring port-forward deployment/prometheus-grafana 3000; done" ENTER'

	echo 'Done setting up monitoring components'

	exit
}
