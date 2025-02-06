#!/bin/bash

NODE=$1

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null 2>&1 && pwd)"

source "$DIR/setup.cfg"

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$NODE" "$1";
}

# Setup KinD node and Knative
server_exec "git clone --branch=$VHIVE_BRANCH $VHIVE_REPO"
server_exec "git clone --depth=1 --branch=$LOADER_BRANCH $LOADER_REPO loader"

# Install Go
server_exec "pushd vhive && bash ./scripts/install_go.sh; source /etc/profile"

# Install Docker
server_exec "curl -fsSL https://get.docker.com -o get-docker.sh"
server_exec "sudo sh get-docker.sh"

# Install kubectl
server_exec 'curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"'
server_exec "sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl"

# Install KinD
server_exec "curl -Lo ./kind https://kind.sigs.k8s.io/dl/v0.26.0/kind-linux-amd64 && chmod +x ./kind && sudo mv ./kind /usr/local/bin/kind"


server_exec "sudo chmod 666 /var/run/docker.sock"

# Run KinD and Knative setup scripts

server_exec "cd loader; bash ./scripts/konk-ci/01-kind.sh"
server_exec "cd loader; sudo bash ./scripts/konk-ci/02-serving.sh"
server_exec "cd loader; sudo bash ./scripts/konk-ci/02-kourier.sh"

server_exec "
INGRESS_HOST=\"127.0.0.1\"
KNATIVE_DOMAIN=\$INGRESS_HOST.sslip.io

kubectl patch configmap -n knative-serving config-domain -p \"{\\\"data\\\": {\\\"\$KNATIVE_DOMAIN\\\": \\\"\\\"}}\"
kubectl patch configmap -n knative-serving config-autoscaler -p \"{\\\"data\\\": {\\\"allow-zero-initial-scale\\\": \\\"true\\\"}}\"
kubectl patch configmap -n knative-serving config-features -p \"{\\\"data\\\": {\\\"kubernetes.podspec-affinity\\\": \\\"enabled\\\"}}\"
kubectl label node knative-control-plane loader-nodetype=worker
"