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
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null 2>&1 && pwd)"

source "$DIR/setup.cfg"

if [ "$CLUSTER_MODE" = "container" ]
then
    OPERATION_MODE="stock-only"
    FIRECRACKER_SNAPSHOTS=""
elif [ $CLUSTER_MODE = "firecracker" ]
then
    OPERATION_MODE="firecracker"
    FIRECRACKER_SNAPSHOTS=""
elif [ $CLUSTER_MODE = "firecracker_snapshots" ]
then
    OPERATION_MODE="firecracker"
    FIRECRACKER_SNAPSHOTS="-snapshots"
else
    echo "Unsupported cluster mode"
    exit 1
fi

if [ $PODS_PER_NODE -gt 1022 ]; then
    # CIDR range limitation exceeded
    echo "Pods per node cannot be greater than 1022. Cluster deployment has been aborted."
    exit 1
fi

if [ "$#" -lt $CONTROL_PLANE_REPLICAS ]; then
    echo "Not enough nodes to set up the requested number of control plane replicas."
    exit 1
fi

if [ "$CONTROL_PLANE_REPLICAS" != 1 ] && [ "$CONTROL_PLANE_REPLICAS" != 3 ] && [ "$CONTROL_PLANE_REPLICAS" != 5 ]; then
    echo "Number of control plane replicas can only be 1, 3, or 5."
    exit 1
fi

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

common_init() {
    internal_init() {
        server_exec $1 "git clone --branch=$VHIVE_BRANCH https://github.com/vhive-serverless/vhive"

        server_exec $1 "pushd ~/vhive/scripts > /dev/null && ./install_go.sh && source /etc/profile && go build -o setup_tool && ./setup_tool setup_node $2 ${OPERATION_MODE}  && popd > /dev/null"
        
        server_exec $1 'tmux new -s containerd -d'
        server_exec $1 'tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER'
        # install precise NTP clock synchronizer
        server_exec $1 'sudo apt-get update && sudo apt-get install -y chrony htop sysstat'
        # synchronize clock across nodes
        server_exec $1 "sudo chronyd -q \"server ops.emulab.net iburst\""
        # dump clock info
        server_exec $1 'sudo chronyc tracking'

        clone_loader $1
        #server_exec $1 '~/loader/scripts/setup/stabilize.sh'
    }

    NODE_COUNTER=1
    for node in "$@"
    do
        # Set up API Server load balancer arguments
        HA_SETTING="REGULAR"
        if [ "$NODE_COUNTER" -eq 1 ]; then
            HA_SETTING="MASTER"
        elif [ "$NODE_COUNTER" -le $CONTROL_PLANE_REPLICAS ]; then
            HA_SETTING="BACKUP"
        fi

        internal_init "$node" $HA_SETTING &
        let NODE_COUNTER++
    done

    wait
}

function setup_master() {
    echo "Setting up master node: $MASTER_NODE"

    server_exec "$MASTER_NODE" 'tmux new -s runner -d'
    server_exec "$MASTER_NODE" 'tmux new -s kwatch -d'
    server_exec "$MASTER_NODE" 'tmux new -s master -d'

    server_exec $MASTER_NODE '~/loader/scripts/setup/rewrite_yaml_files.sh'

<<<<<<< HEAD
    MN_CLUSTER="pushd ~/vhive/scripts > /dev/null && ./setup_tool create_multinode_cluster ${OPERATION_MODE} && popd > /dev/null"
=======
    MN_CLUSTER="pushd ~/vhive/scripts > /dev/null && ./setup_tool create_multinode_cluster ${OPERATION_MODE} ${CONTROL_PLANE_REPLICAS} && popd > /dev/null"
>>>>>>> 78e9b94 (K8s high-availability mode)
    server_exec "$MASTER_NODE" "tmux send -t master \"$MN_CLUSTER\" ENTER"

    # Get the join token from k8s.
    while ! server_exec "$MASTER_NODE" "[ -e ~/vhive/scripts/masterKey.yaml ]"; do
        sleep 1
    done

    MASTER_LOGIN_TOKEN=$(server_exec "$MASTER_NODE" \
        'awk '\''/^ApiserverAdvertiseAddress:/ {ip=$2} \
        /^ApiserverPort:/ {port=$2} \
        /^ApiserverToken:/ {token=$2} \
        /^ApiserverDiscoveryToken:/ {discovery_token=$2} \
        /^ApiserverCertificateKey:/ {certificate_key=$2} \
        END {print "sudo kubeadm join " ip ":" port " --token " token " --discovery-token-ca-cert-hash " discovery_token " --control-plane --certificate-key " certificate_key}'\'' ~/vhive/scripts/masterKey.yaml')

    LOGIN_TOKEN=$(server_exec "$MASTER_NODE" \
        'awk '\''/^ApiserverAdvertiseAddress:/ {ip=$2} \
        /^ApiserverPort:/ {port=$2} \
        /^ApiserverToken:/ {token=$2} \
        /^ApiserverDiscoveryToken:/ {token_hash=$2} \
        END {print "sudo kubeadm join " ip ":" port " --token " token " --discovery-token-ca-cert-hash " token_hash}'\'' ~/vhive/scripts/masterKey.yaml')

    server_exec $MASTER_NODE "kubectl taint nodes \$(hostname) node-role.kubernetes.io/control-plane-"
    server_exec $MASTER_NODE "kubectl label nodes \$(hostname) loader-nodetype=master"
}

function setup_vhive_firecracker_daemon() {
    node=$1

    server_exec $node 'cd vhive; source /etc/profile && go build'
    server_exec $node 'tmux new -s firecracker -d'
    server_exec $node 'tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee ~/firecracker_log.txt" ENTER'
    server_exec $node 'tmux new -s vhive -d'
    server_exec $node 'tmux send -t vhive "cd vhive" ENTER'
    RUN_VHIVE_CMD="sudo ./vhive ${FIRECRACKER_SNAPSHOTS} 2>&1 | tee ~/vhive_log.txt"
    server_exec $node "tmux send -t vhive \"$RUN_VHIVE_CMD\" ENTER"
}

function setup_workers() {
    internal_setup() {
        node=$1

        echo "Setting up worker node: $node"
        
        server_exec $node "pushd ~/vhive/scripts > /dev/null && ./setup_tool setup_worker_kubelet ${OPERATION_MODE} && popd > /dev/null"

        if [ "$OPERATION_MODE" = "firecracker" ]; then
            setup_vhive_firecracker_daemon $node
        fi

        if [ "$2" = "MASTER" ]; then
            server_exec $node "sudo ${MASTER_LOGIN_TOKEN}"
            server_exec $node "kubectl taint nodes \$(hostname) node-role.kubernetes.io/control-plane-"
            server_exec $node "kubectl label nodes \$(hostname) loader-nodetype=master"
            echo "Backup master node $node has joined the cluster."
        else
            server_exec $node "sudo ${LOGIN_TOKEN}"

            if [ "$3" = "LOADER" ]; then
                # First node after the control plane nodes
                server_exec $node "kubectl label nodes \$(hostname) loader-nodetype=monitoring" < /dev/null
            else
                server_exec $node "kubectl label nodes \$(hostname) loader-nodetype=worker" < /dev/null
            fi

            echo "Worker node $node has joined the cluster."
        fi

        # Stretch the capacity of the worker node to 240 (k8s default: 110)
        # Empirically, this gives us a max. #pods being 240-40=200
        #echo "Stretching node capacity for $node."
        #server_exec $node "echo \"maxPods: ${PODS_PER_NODE}\" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)"
        #server_exec $node "echo \"containerLogMaxSize: 512Mi\" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)"
        #server_exec $node 'sudo systemctl restart kubelet'
        #server_exec $node 'sleep 10'

        # Rejoin has to be performed although errors will be thrown. Otherwise, restarting the kubelet will cause the node unreachable for some reason
        #if [ $2 -eq "MASTER" ]; then
        #    server_exec $node "sudo ${MASTER_LOGIN_TOKEN} > /dev/null 2>&1"
        #    echo "Backup master node $node joined the cluster (again :P)."
        #else
        #    server_exec $node "sudo ${LOGIN_TOKEN} > /dev/null 2>&1"
        #    echo "Worker node $node joined the cluster (again :P)."
        #fi
    }

    NODE_COUNTER=1
    for node in "$@"
    do
        # Set up API Server load balancer arguments - Less than because 1 CP is the "main" master node already
        HA_SETTING="OTHER"
        LOADER_NODE="OTHER"

        if [ "$NODE_COUNTER" -lt $CONTROL_PLANE_REPLICAS ]; then
            HA_SETTING="MASTER"
        elif [ "$NODE_COUNTER" -eq $CONTROL_PLANE_REPLICAS ]; then
            LOADER_NODE="LOADER"
        fi

        internal_setup "$node" "$HA_SETTING" "$LOADER_NODE" &
        let NODE_COUNTER++
    done

    wait
}

function setup_fakes() {
    # Compute variables locally
    KWOK_REPO=kubernetes-sigs/kwok
    KWOK_LATEST_RELEASE=$(curl "https://api.github.com/repos/${KWOK_REPO}/releases/latest" | jq -r '.tag_name')

    echo "Installing kwok $KWOK_LATEST_RELEASE"

    # Install kwok
    # server_exec $MASTER_NODE "source /etc/profile; go install sigs.k8s.io/kwok/cmd/{kwok,kwokctl}@${KWOK_LATEST_RELEASE}"

    # Deploy kwok in a Cluster
    server_exec $MASTER_NODE "kubectl apply -f \"https://github.com/$KWOK_REPO/releases/download/$KWOK_LATEST_RELEASE/kwok.yaml\""
    server_exec $MASTER_NODE "kubectl apply -f \"https://github.com/$KWOK_REPO/releases/download/$KWOK_LATEST_RELEASE/stage-fast.yaml\""
    server_exec $MASTER_NODE "kubectl apply -f \"https://github.com/$KWOK_REPO/releases/download/$KWOK_LATEST_RELEASE/metrics-usage.yaml\""
    server_exec $MASTER_NODE "cd loader; kubectl patch configmap config-features -n knative-serving -p '{\"data\": {\"kubernetes.podspec-tolerations\": \"enabled\"}}'"

    # Deploy kwok fake nodes
    server_exec $MASTER_NODE "cd loader; for i in {1..$KWOK_NODE_NUM}; do KWOK_NODE_NAME=node-fake-\$i envsubst < config/kwok_fake_node.yaml | kubectl apply -f -; done"

    # Deploy timer service
    server_exec $MASTER_NODE 'kubectl create namespace kwok-system'
    server_exec $MASTER_NODE 'cd loader; kubectl apply -f config/deploy_timer.yaml'
}

function extend_CIDR() {
    #* Get node name list.
    readarray -t NODE_NAMES < <(server_exec $MASTER_NODE 'kubectl get no' | tail -n +2 | awk '{print $1}')

    if [ ${#NODE_NAMES[@]} -gt 63 ]; then
        echo "Cannot extend CIDR range for more than 63 nodes. Cluster deployment has been aborted."
        exit 1
    fi

    for i in "${!NODE_NAMES[@]}"; do
        NODE_NAME=${NODE_NAMES[i]}
        #* Compute subnet: 00001010.10101000.000000 00.00000000 -> about 1022 IPs per worker.
        #* To be safe, we change both master and workers with an offset of 0.0.4.0 (4 * 2^8)
        # (NB: zsh indices start from 1.)
        #* Assume less than 63 nodes in total.
        let SUBNET=i*4+4
        #* Extend pod ip range, delete and create again.
        server_exec $MASTER_NODE "kubectl get node $NODE_NAME -o json | jq '.spec.podCIDR |= \"10.168.$SUBNET.0/22\"' > node.yaml"
        server_exec $MASTER_NODE "kubectl delete node $NODE_NAME && kubectl create -f node.yaml"

        echo "Changed pod CIDR for worker $NODE_NAME to 10.168.$SUBNET.0/22"
        sleep 5
    done

    #* Join the cluster for the 3rd time.
    for node in "$@"
    do
        server_exec $node "sudo ${LOGIN_TOKEN} > /dev/null 2>&1"
        echo "Worker node $node joined the cluster (again^2 :D)."
    done
}

function clone_loader() {
    server_exec $1 "git clone --depth=1 --branch=$LOADER_BRANCH https://github.com/vhive-serverless/invitro.git loader"
    server_exec $1 'echo -en "\n\n" | sudo apt-get install -y python3-pip python-dev'
    server_exec $1 'cd; cd loader; pip install -r config/requirements.txt'
}

function copy_k8s_certificates() {
    function internal_copy() {
        server_exec $1 "mkdir -p ~/.kube"
        rsync ./kubeconfig $1:~/.kube/config
    }

    echo $MASTER_NODE
    rsync $MASTER_NODE:~/.kube/config ./kubeconfig

    for node in "$@"
    do
        internal_copy "$node" &
    done

    wait

    rm ./kubeconfig
}

###############################################
######## MAIN SETUP PROCEDURE IS BELOW ########
###############################################

{
    # Set up all nodes including the master
    common_init "$@"

    shift # make argument list only contain worker nodes (drops master node)

    setup_master

    # Copy API server certificates from master to each worker node
    copy_k8s_certificates "$@"

    # Join cluster
    setup_workers "$@"

    if [ $PODS_PER_NODE -gt 240 ]; then
        extend_CIDR "$@"
    fi

    # Notify the master that all nodes have joined the cluster
    server_exec $MASTER_NODE 'tmux send -t master "y" ENTER'

    namespace_info=$(server_exec $MASTER_NODE "kubectl get namespaces")
    while [[ ${namespace_info} != *'knative-serving'*  ]]; do
        sleep 60
        namespace_info=$(server_exec $MASTER_NODE "kubectl get namespaces")
    done

    server_exec $MASTER_NODE 'cd loader; bash scripts/setup/patch_init_scale.sh'

    # patch knative to accept nodeselector
    server_exec $MASTER_NODE "cd loader; kubectl patch configmap config-features -n knative-serving -p '{\"data\": {\"kubernetes.podspec-nodeselector\": \"enabled\"}}'"
    
    # update limits
    server_exec $MASTER_NODE "kubectl patch deployment istio-ingressgateway -n istio-system --patch \
        '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"istio-proxy\",\"resources\":{\"limits\":{\"cpu\":\"10\", \"memory\":\"20Gi\"}}}]}}}}'"
    server_exec $MASTER_NODE "kubectl patch deployment cluster-local-gateway -n istio-system --patch \
        '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"istio-proxy\",\"resources\":{\"limits\":{\"memory\":\"20Gi\"}}}]}}}}'"
    server_exec $MASTER_NODE "kubectl patch deployment coredns -n kube-system --patch \
        '{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"name\":\"coredns\",\"resources\":{\"limits\":{\"memory\":\"20Gi\"}}}]}}}}'"

    if [[ "$DEPLOY_PROMETHEUS" == true ]]; then
        $DIR/expose_infra_metrics.sh $MASTER_NODE
    fi

    # Check if ENABLE_KWOK is set to true
    if [ "$ENABLE_KWOK" = true ]; then
        setup_fakes
    fi
}
