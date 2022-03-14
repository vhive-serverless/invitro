#!/usr/bin/env bash
MASTER_NODE=$1

DIR="$(pwd)/scripts/setup/"

source "$DIR/setup.cfg"

server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2";
}

#* Run initialisation on a node.
common_init() {
	server_exec $1 "git clone --branch=$VHIVE_BRANCH https://github.com/ease-lab/vhive"
	server_exec $1 "cd; ./vhive/scripts/cloudlab/setup_node.sh stock-only"
	server_exec $1 'tmux new -s containerd -d'
	server_exec $1 'tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER'
}

#* Set up all nodes including the master.
for node in "$@"
do
	echo $node
	common_init "$node" &
done
wait $!

LOGIN_TOKEN=""
IS_MASTER=1
for node in "$@"
do
	server_exec() { 
		ssh -oStrictHostKeyChecking=no -p 22 "$node" "$1"; 
	}
	if [ $IS_MASTER -eq 1 ]
	then
		echo "Setting up master node: $node"
		IS_MASTER=0
		server_exec 'wget -q https://dl.google.com/go/go1.17.linux-amd64.tar.gz >/dev/null'
		server_exec 'sudo rm -rf /usr/local/go && sudo tar -C /usr/local/ -xzf go1.17.linux-amd64.tar.gz >/dev/null'
		# server_exec 'sudo apt-get install libcairo2-dev libjpeg-dev libgif-dev'
		server_exec 'echo "export PATH=$PATH:/usr/local/go/bin" >> .profile'
		
		server_exec 'tmux new -s runner -d'
		server_exec 'tmux new -s kwatch -d'
		server_exec 'tmux new -s master -d'
		server_exec 'tmux send -t master "./vhive/scripts/cluster/create_multinode_cluster.sh stock-only" ENTER'
		
		# Get the join token from k8s.
		while [ ! "$LOGIN_TOKEN" ]
		do
			sleep 1s
			server_exec 'tmux capture-pane -t master -b token'
			LOGIN_TOKEN="$(server_exec 'tmux show-buffer -b token | grep -B 3 "All nodes need to be joined"')"
		done
		# cut of last line
		LOGIN_TOKEN=${LOGIN_TOKEN%[$'\t\r\n']*}
		# remove the \
		LOGIN_TOKEN=${LOGIN_TOKEN/\\/}
		# remove all remaining tabs, line ends and returns
		LOGIN_TOKEN=${LOGIN_TOKEN//[$'\t\r\n']}
		
	else
		echo "Setting up worker node: $node"
		server_exec "./vhive/scripts/cluster/setup_worker_kubelet.sh stock-only"

		#* We don't need vhive in container mode.
		# server_exec 'cd vhive; source /etc/profile && go build'
		# server_exec 'tmux new -s firecracker -d'
		# server_exec 'tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee ~/firecracker_log.txt" ENTER'
		# server_exec 'tmux new -s vhive -d'
		# server_exec 'tmux send -t vhive "cd vhive" ENTER'
		# server_exec 'tmux send -t vhive "sudo ./vhive 2>&1 | tee ~/vhive_log.txt" ENTER'
	fi
done

IS_MASTER=1
for node in "$@"
do
	if [ $IS_MASTER -eq 1 ]
	then
		IS_MASTER=0
	else
		ssh -oStrictHostKeyChecking=no -p 22 $node "sudo ${LOGIN_TOKEN}"
		echo "Worker node $node joined the cluster."

		#* Stretch the capacity of the worker node to 500 (k8s default: 110).
		echo "Streching node capacity for $node."
		server_exec 'echo "maxPods: 500" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)'
		server_exec 'sudo systemctl restart kubelet'
		
		#* Rejoin has to be performed although errors will be thrown. Otherwise, restarting the kubelet will cause the node unreachable for some reason.
		ssh -oStrictHostKeyChecking=no -p 22 $node "sudo ${LOGIN_TOKEN} > /dev/null 2>&1"
		echo "Worker node $node joined the cluster (again :P)."
	fi
done

server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

#* Notify the master that all nodes have been registered
server_exec 'tmux send -t master "y" ENTER'
echo "Master node $MASTER_NODE finalised." 

#* Setup github authentication.
ACCESS_TOKEH="$(cat $GITHUB_TOKEN)"

server_exec 'echo -en "\n\n" | ssh-keygen -t rsa'
server_exec 'ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts'

server_exec 'curl -H "Authorization: token '"$ACCESS_TOKEH"'" --data "{\"title\":\"'"key:\$(hostname)"'\",\"key\":\"'"\$(cat ~/.ssh/id_rsa.pub)"'\"}" https://api.github.com/user/keys'

#* Get loader and dependencies.
server_exec "git clone --branch=$LOADER_BRANCH git@github.com:eth-easl/loader.git"
server_exec 'echo -en "\n\n" | sudo apt-get install python3-pip python-dev'
server_exec 'cd; cd loader; pip install -r config/requirements.txt'

$DIR/expose_infra_metrics.sh $MASTER_NODE

#* Disable turbo boost.
server_exec 'bash loader/scripts/setup/turbo_boost.sh disable'
#* Disable hyperthreading.
server_exec 'echo off | sudo tee /sys/devices/system/cpu/smt/control'

echo "Logging in master node $MASTER_NODE"
ssh -p 22 $MASTER_NODE
