#!/usr/bin/env bash

MASTER_NODE=$1
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null 2>&1 && pwd)"

source "$DIR/setup.cfg"

if [ "$CLUSTER_MODE" = "container" ]
then
    OPERATION_MODE="stock-only"
    FIRECRACKER_SNAPSHOTS=""
elif [ $CLUSTER_MODE = "firecracker" ]
then
    OPERATION_MODE=""
    FIRECRACKER_SNAPSHOTS=""
elif [ $CLUSTER_MODE = "firecracker_snapshots" ]
then
    OPERATION_MODE=""
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

server_exec() {
    ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

#* Run initialisation on a node.
common_init() {
    server_exec $1 "git clone --branch=$VHIVE_BRANCH https://github.com/ease-lab/vhive"
    server_exec $1 "cd; ./vhive/scripts/cloudlab/setup_node.sh $OPERATION_MODE"
    server_exec $1 'tmux new -s containerd -d'
    server_exec $1 'tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER'
    server_exec $1 'sudo ntpdate -s ops.emulab.net' # synchronize clock across nodes
}

function setup_master() {
    echo "Setting up master node: $MASTER_NODE"

    server_exec "$MASTER_NODE" 'wget -q https://dl.google.com/go/go1.17.linux-amd64.tar.gz >/dev/null'
    server_exec "$MASTER_NODE" 'sudo rm -rf /usr/local/go && sudo tar -C /usr/local/ -xzf go1.17.linux-amd64.tar.gz >/dev/null'
    server_exec "$MASTER_NODE" 'echo "export PATH=$PATH:/usr/local/go/bin" >> .profile'

    server_exec "$MASTER_NODE" 'tmux new -s runner -d'
    server_exec "$MASTER_NODE" 'tmux new -s kwatch -d'
    server_exec "$MASTER_NODE" 'tmux new -s master -d'

    # Rewrite the YAML files for Knative and Istio in the vHive repo
    server_exec "$MASTER_NODE" 'tmux send -t master "cd; ./loader/scripts/setup/rewrite_yaml_files.sh" ENTER'

    MN_CLUSTER="cd; ./vhive/scripts/cluster/create_multinode_cluster.sh ${OPERATION_MODE}"
    server_exec "$MASTER_NODE" "tmux send -t master \"$MN_CLUSTER\" ENTER"

    # Get the join token from k8s.
    while [ ! "$LOGIN_TOKEN" ]
    do
        sleep 1s
        server_exec "$MASTER_NODE" 'tmux capture-pane -t master -b token'
        LOGIN_TOKEN="$(server_exec "$MASTER_NODE" 'tmux show-buffer -b token | grep -B 3 "All nodes need to be joined"')"
        echo "$LOGIN_TOKEN"
    done
    # cut of last line
    LOGIN_TOKEN=${LOGIN_TOKEN%[$'\t\r\n']*}
    # remove the \
    LOGIN_TOKEN=${LOGIN_TOKEN/\\/}
    # remove all remaining tabs, line ends and returns
    LOGIN_TOKEN=${LOGIN_TOKEN//[$'\t\r\n']}
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
    for node in "$@"
    do
        echo "Setting up worker node: $node"
        server_exec $node "./vhive/scripts/cluster/setup_worker_kubelet.sh $OPERATION_MODE"

        if [ "$OPERATION_MODE" = "" ]; then
            setup_vhive_firecracker_daemon $node
        fi

        server_exec $node "sudo ${LOGIN_TOKEN}"
        echo "Worker node $node has joined the cluster."

        #* Disable worker turbo boost for better timing.
        server_exec $node './vhive/scripts/utils/turbo_boost.sh disable'
        #* Disable worker hyperthreading (security).
        server_exec $node 'echo off | sudo tee /sys/devices/system/cpu/smt/control'

        #* Stretch the capacity of the worker node to 240 (k8s default: 110).
        #* Empirically, this gives us a max. #pods being 240-40=200.
        echo "Streching node capacity for $node."
        server_exec $node "echo \"maxPods: ${PODS_PER_NODE}\" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)"
        server_exec $node 'sudo systemctl restart kubelet'
        server_exec $node 'sleep 10'

        #* Rejoin has to be performed although errors will be thrown. Otherwise, restarting the kubelet will cause the node unreachable for some reason.
        server_exec $node "sudo ${LOGIN_TOKEN} > /dev/null 2>&1"
        echo "Worker node $node joined the cluster (again :P)."
    done
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

###############################################
######## MAIN SETUP PROCEDURE IS BELOW ########
###############################################

{
    #* Set up all nodes including the master.
    for node in "$@"
    do
        echo $node
        common_init "$node" &
    done
    wait $!

    shift # make argument list only contain worker nodes (drops master node)

    #* Get loader and dependencies.
    server_exec $MASTER_NODE "git clone --branch=$LOADER_BRANCH git@github.com:eth-easl/loader.git"
    server_exec $MASTER_NODE 'echo -en "\n\n" | sudo apt-get install python3-pip python-dev'
    server_exec $MASTER_NODE 'cd; cd loader; pip install -r config/requirements.txt'

    setup_master
    setup_workers "$@"

    if [ $PODS_PER_NODE -gt 240 ]; then
        extend_CIDR "$@"
    fi

    #* Notify the master that all nodes have been registered
    server_exec $MASTER_NODE 'tmux send -t master "y" ENTER'
    echo "Master node $MASTER_NODE finalised."

    #* Setup github authentication.
    ACCESS_TOKEH="$(cat $GITHUB_TOKEN)"

    server_exec $MASTER_NODE 'echo -en "\n\n" | ssh-keygen -t rsa'
    server_exec $MASTER_NODE 'ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts'

    server_exec $MASTER_NODE 'curl -H "Authorization: token '"$ACCESS_TOKEH"'" --data "{\"title\":\"'"key:\$(hostname)"'\",\"key\":\"'"\$(cat ~/.ssh/id_rsa.pub)"'\"}" https://api.github.com/user/keys'

    source $DIR/taint.sh

    # force placement of metrics collectors and instrumentation on the master node
    taint_workers $MASTER_NODE
    $DIR/expose_infra_metrics.sh $MASTER_NODE
    untaint_workers $MASTER_NODE

    #* Disable master turbo boost.
    server_exec $MASTER_NODE './vhive/scripts/utils/turbo_boost.sh disable'
    #* Disable master hyperthreading.
    server_exec $MASTER_NODE 'echo off | sudo tee /sys/devices/system/cpu/smt/control'
    #* Create CGroup.
    server_exec $MASTER_NODE 'sudo bash loader/scripts/isolation/define_cgroup.sh'

    taint_master $MASTER_NODE
}
