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

label_master() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    if [[ $TYPE == *"master"* ]]; then
      echo "Label ${NODE}"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=monitoring" < /dev/null
    fi
  done < tmp

  rm tmp
}

label_workers() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    if [[ $TYPE != *"master"* ]]; then
      echo "Label ${NODE}"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    fi
  done < tmp

  rm tmp
}

label_all_workers() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)

    echo "Label ${NODE}"
    server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
  done < tmp

  rm tmp
}
