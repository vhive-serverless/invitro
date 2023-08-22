#!/bin/bash

######################################################
# Script for labeling cluster
######################################################
# label_master $MASTER_NODE - label master node as monitoring
# label_workers $MASTER_NODE - label worker node as worker
######################################################

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $1 $2;
}

label_nodes() {
  MASTER_NODE=$1
  LOADER_NODE=$2
  LOADER_NODE_NAME="$(server_exec "$LOADER_NODE" hostname)"
  echo $LOADER_NODE_NAME

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    echo "Label ${NODE}"
    if [[ $TYPE == *"master"* ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master" < /dev/null
    elif [[ $NODE == $LOADER_NODE_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=monitoring" < /dev/null
    else
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    fi
  done < tmp

  rm tmp
}
