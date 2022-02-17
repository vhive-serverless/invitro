#!/bin/bash

# get config
source setup_config

MODE=''
MASTER_INDEX=1
LOGIN_TOKEN=""

send() { ssh -oStrictHostKeyChecking=no -p 22 "$1" "$2"; }

# function to initialize everything that needs to run on all servers
common_init() {
		send "$1" "git clone https://github.com/ease-lab/vhive"
		send "$1" "cd vhive; git checkout bote"
		send "$1" './vhive/scripts/cloudlab/setup_node.sh' $MODE
		send "$1" 'tmux new -s containerd -d'
		send "$1" 'tmux send -t containerd "sudo containerd 2>&1 | tee ~/containerd_log.txt" ENTER'
	}

# common init on all servers and wait until all of them are finished
for server in "$@"
do
		common_init "$server" &
done
wait

# iterate over inputs, first one is master rest will be registered as workers
for server in "$@"
do
		# echo "$server"
		send() { ssh -oStrictHostKeyChecking=no -p 22 "$server" "$1"; }
		if [ $MASTER_INDEX -eq 1 ]
		then
				MASTER_INDEX=0
				send 'sudo apt -y install moreutils'
				send 'wget -q https://dl.google.com/go/go1.17.linux-amd64.tar.gz'
				send 'sudo rm -rf /usr/local/go && sudo tar -C /usr/local/ -xzf go1.17.linux-amd64.tar.gz'
				send 'echo "export PATH=$PATH:/usr/local/go/bin" >> .profile'
				send 'tmux new -s vhive -d'
				send 'tmux send -t vhive ./vhive/scripts/cluster/create_multinode_cluster.sh '"$MODE"' ENTER'
				# wait for the registering token to appear
				while [ ! "$LOGIN_TOKEN" ]
				do
						sleep 1s
						send 'tmux capture-pane -t vhive -b token'
						LOGIN_TOKEN="$(send 'tmux show-buffer -b token | grep -B 3 "All nodes need to be joined"')"
				done
				# cut of last line
				LOGIN_TOKEN=${LOGIN_TOKEN%[$'\t\r\n']*}
				# remove the \
				LOGIN_TOKEN=${LOGIN_TOKEN/\\/}
				# remove all remaining tabs, line ends and returns
				LOGIN_TOKEN=${LOGIN_TOKEN//[$'\t\r\n']}
		else
				send './vhive/scripts/cluster/setup_worker_kubelet.sh' $MODE
				send 'cd vhive; source /etc/profile && go build'
				send 'tmux new -s firecracker -d'
				send 'tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee ~/firecracker_log.txt" ENTER'
				send 'tmux new -s vhive -d'
				send 'tmux send -t vhive "cd vhive" ENTER'
				send 'tmux send -t vhive "sudo ./vhive 2>&1 | tee ~/vhive_log.txt" ENTER'
		fi
done

MASTER_INDEX=1
for server in "$@"
do
		if [ $MASTER_INDEX -eq 1 ]
		then
				MASTER_INDEX=0
		else
				send() { ssh -oStrictHostKeyChecking=no -p 22 "$server" "$1"; }
				send "sudo ${LOGIN_TOKEN}"
		fi
done

masterServer="$1"
masterSend() { ssh -oStrictHostKeyChecking=no -p 22 "$masterServer" "$1"; }
# # notify the master that all nodes have been registered
masterSend 'tmux send -t vhive "y" ENTER'

# # set up monitoring on master
# # set up name variable has no further meaning than an ID
# releaseName="metrics"
# # install helm
# masterSend 'curl https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | bash'
# # install and start prometheus stack using helm
# masterSend 'helm repo add prometheus-community https://prometheus-community.github.io/helm-charts'
# masterSend 'helm repo update'
# masterSend 'kubectl create namespace monitoring'
# masterSend "helm install -n monitoring --version 17.1.0 $releaseName prometheus-community/kube-prometheus-stack"
# # wait a few seconds for it to finished
# sleep 5s
# # set up port forwarding
# masterSend 'tmux new -s prometheus -d'
# portForwardCommand="kubectl port-forward -n monitoring service/"$releaseName"-kube-prometheus-st-prometheus 9090"
# portForwardCommand=\'$portForwardCommand\'
# sleep 15s
# masterSend "tmux send -t prometheus $portForwardCommand ENTER"

# # set the prometheus to find all service monitors
# masterSend "sudo kubectl -n monitoring patch prometheus "$releaseName"-kube-prometheus-st-prometheus --type json -p '[{"op": "replace", "path": "/spec/serviceMonitorSelector", "value": {}}, {"op": "replace", "path": "/spec/serviceMonitorNamspaceSelector", "value": {}}]'"
# # set the node exporter speed to 2s
# masterSend "sudo kubectl -n monitoring patch ServiceMonitor "$releaseName"-kube-prometheus-st-node-exporter --type json -p '[{"op": "add", "path": "/spec/endpoints/0/interval", "value": "2s"}]'"
# # set up the knative service monitors
# scp monitors.yaml "$1":
# masterSend 'kubectl apply -f monitors.yaml'

# notes:
# grafana id: admin pw: prom-operator
