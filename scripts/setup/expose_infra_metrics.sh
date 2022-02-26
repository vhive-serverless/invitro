#!/bin/bash
MASTER_NODE=$1
server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

echo 'Setting up monitoring components'
#* Deploy Metrics Server to k8s in namespace kube-system (`kubectl get all -n kube-system -owide`)
server_exec 'cd loader; kubectl apply -f config/metrics-server-components.yaml'

#* Install helm
server_exec 'curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash'
#* Install and start prometheus stack using helm
server_exec 'helm repo add prometheus-community https://prometheus-community.github.io/helm-charts'
server_exec 'helm repo update'
server_exec 'kubectl create namespace monitoring'

release_label="prometheus"
server_exec "cd loader; helm install -n monitoring $release_label prometheus-community/kube-prometheus-stack -f config/prometh_values_kn.yaml"
#* Apply the ServiceMonitors/PodMonitors to collect metrics from Knative.
server_exec 'cd loader; kubectl apply -f config/prometh_kn.yaml'

#* Set up port forwarding
server_exec 'tmux new -s prometheusd -d'
server_exec 'tmux send -t prometheusd "kubectl port-forward -n monitoring svc/prometheus-operated 9090" ENTER'

#* Set up grafana dash board (id: admin, pwd: prom-operator)
server_exec 'tmux new -s grafanad -d'
server_exec 'tmux send -t grafanad "kubectl -n monitoring port-forward deployment/prometheus-grafana 3000" ENTER'
