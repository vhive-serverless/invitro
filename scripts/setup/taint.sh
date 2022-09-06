#!/bin/bash

######################################################
# Script for tainting cluster
######################################################
# taint_master $MASTER_NODE - forbid scheduling pods on nodes labeled as master nodes
# taint_workers $MASTER_NODE - forbid scheduling pods on any node not labeled as master
# untaint_workers $MASTER_NODE - remove taint introduced by taint_worker function
######################################################

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $1 $2;
}

taint_master() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    if [[ $TYPE == *"master"* ]]; then
      echo "Tainted ${NODE}"
      server_exec $MASTER_NODE "kubectl taint nodes ${NODE} key1=value1:NoSchedule" < /dev/null
    fi
  done < tmp

  rm tmp
}

taint_workers() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    if [[ $TYPE != *"master"* ]]; then
      echo "Tainted ${NODE}"
      server_exec $MASTER_NODE "kubectl taint nodes ${NODE} key1=value1:NoSchedule" < /dev/null
    fi
  done < tmp

  rm tmp
}

untaint_workers() {
  MASTER_NODE=$1

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i '1d' tmp

  while read LINE; do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    if [[ $TYPE != *"master"* ]]; then
      echo "Untainted ${NODE}"
      server_exec $MASTER_NODE "kubectl taint nodes ${NODE} key1-" < /dev/null
    fi
  done < tmp

  rm tmp
}
