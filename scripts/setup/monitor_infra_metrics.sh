
MASTER_NODE=$1
server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

echo 'Setting up monitoring components'
# #* Deploy Metrics Server to k8s in namespace `kube-system`
server_exec 'cd loader; kubectl apply -f config/metrics-server-components.yaml'

#* Install helm
server_exec 'curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash'
#* Install and start prometheus stack using helm
server_exec 'helm repo add prometheus-community https://prometheus-community.github.io/helm-charts'
server_exec 'helm repo update'
server_exec 'kubectl create namespace monitoring'

prefix="loader"
server_exec "helm install -n monitoring $prefix prometheus-community/kube-prometheus-stack"
sleep 5s

# #* Set up port forwarding
server_exec 'tmux new -s prometheusd -d'
server_exec 'tmux send -t prometheusd "kubectl port-forward -n service/monitoring service/loader-kube-prometheus-st-prometheus 9090" ENTER'

#* Find all service monitors
server_exec "sudo kubectl -n monitoring patch prometheus loader-kube-prometh-prometheus --type json -p '[{"op": "replace", "path": "/spec/serviceMonitorSelector", "value": {}}, {"op": "replace", "path": "/spec/serviceMonitorNamspaceSelector", "value": {}}]'"
#* Set the node exporter speed to 500ms
server_exec "sudo kubectl -n monitoring patch ServiceMonitor loader-prometheus-node-exporter --type json -p '[{"op": "add", "path": "/spec/endpoints/0/interval", "value": "0.5s"}]'"
#* Set up service monitors for knative
server_exec 'cd loader; kubectl apply -f config/prometh_kn.yaml'

#* Set up grafana dash board (id: admin, pwd: prom-operator)
server_exec 'tmux new -s grafanad -d'
server_exec 'tmux send -t grafanad "kubectl -n monitoring port-forward deployment/loader-grafana 3000" ENTER'
