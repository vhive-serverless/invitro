#!/usr/bin/env bash
MASTER_NODE=$1

server_exec() { 
	ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

{
	echo 'Setting up fake nvidia plugin'
	rsync config/nvidia-device-plugin.yml $MASTER_NODE:~/nvidia-device-plugin.yml
	server_exec 'kubectl apply -f ~/nvidia-device-plugin.yml'

	server_exec 'kubectl get pods -n kube-system'

	echo 'Done setting up fake nvidia plugin'
	
	exit
}
