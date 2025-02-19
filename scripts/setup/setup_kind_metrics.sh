#!/bin/bash

set -e

NODE=$(kubectl get nodes --show-labels --no-headers -o wide | grep name="knative-control-plane" | awk '{print $6}')
server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $NODE $1;
}
# Install git
server_exec 'sudo apt update'
server_exec 'sudo apt install -y git-all'

# Download and install yq
sudo curl -L "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64" -o /usr/local/bin/yq
sudo chmod +x /usr/local/bin/yq

# Install tmux
server_exec 'sudo apt install -y tmux'

# Install helm
curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Add prometheus to local helm repository
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Create namespace monitoring to deploy all services in that namespace
kubectl create namespace monitoring

# Edit config file
yq eval '
  .prometheus.prometheusSpec.resources.limits.cpu = "500m" |
  .prometheus.prometheusSpec.resources.limits.memory = "512Mi" |
  .prometheus.prometheusSpec.resources.requests.cpu = "100m" |
  .prometheus.prometheusSpec.resources.requests.memory = "256Mi"
' -i "./config/prometh_values_kn.yaml"

# Install prometheus stack
release_label="prometheus"
prometheus_chart_version="60.1.0"

helm install \
    -n monitoring $release_label \
    --version $prometheus_chart_version prometheus-community/kube-prometheus-stack \
    -f ./config/prometh_values_kn.yaml

# Config kubectl config
docker exec knative-control-plane sh -c "mkdir -p /home/$(whoami)/.kube"
docker cp ~/.kube/config knative-control-plane:/home/$(whoami)/.kube/config
docker exec knative-control-plane sh -c "echo 'export KUBECONFIG=/home/$(whoami)/.kube/config' >> /home/$(whoami)/.bashrc"
docker exec knative-control-plane sh -c "sudo chown $(whoami):$(whoami) /home/$(whoami)/.kube/config"
docker exec knative-control-plane sh -c "sed -i 's#https://127\.0\.0\.1:[0-9]\{1,\}#https://127.0.0.1:6443#g' /home/$(whoami)/.kube/config"

# wait for pods to be ready
i=0
max_retries=10 

while kubectl get pods -n monitoring | awk 'NR>1 {print $3}' | grep -qv Running; do
    echo "Waiting for pods to be ready ($i/$max_retries)"
    kubectl get pods -n monitoring
    sleep 5
    ((i+=1))

    if [ "$i" -eq "$max_retries" ]; then
        echo "Timeout waiting for pods to be ready. Exiting..."
        exit 1
    fi
done

# port-forward prometheus to localhost:9090
server_exec 'tmux new -s prometheusd -d'
server_exec 'tmux send -t prometheusd "while true; do kubectl port-forward -n monitoring svc/prometheus-operated 9090; done" ENTER'