#!/bin/bash

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
  KNATIVE_NODE=$3
  AUTOSCALER_NODE=$4
  INGRESS_NODE=$5
  WORKER_NODE_1=$6
  WORKER_NODE_2=$7
  WORKER_NODE_3=$8

  LOADER_NODE_NAME="$(server_exec "$LOADER_NODE" hostname)"
  KNATIVE_NODE_NAME="$(server_exec "$KNATIVE_NODE" hostname)"
  AUTOSCALER_NODE_NAME="$(server_exec "$AUTOSCALER_NODE" hostname)"
  INGRESS_NODE_NAME="$(server_exec "$INGRESS_NODE" hostname)"
  WORKER_NODE_1_NAME="$(server_exec "$WORKER_NODE_1" hostname)"
  WORKER_NODE_2_NAME="$(server_exec "$WORKER_NODE_2" hostname)"
  WORKER_NODE_3_NAME="$(server_exec "$WORKER_NODE_3" hostname)"

  echo $LOADER_NODE_NAME

  server_exec $MASTER_NODE 'kubectl get nodes' | tail +2 | while IFS= read -r LINE
  do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    echo "Label ${NODE}"
    if [[ $TYPE == *"control-plane"* ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master" < /dev/null
    elif [[ $NODE == $LOADER_NODE_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=monitoring" < /dev/null
    elif [[ $NODE == $KNATIVE_NODE_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master-knative" < /dev/null
    elif [[ $NODE == $AUTOSCALER_NODE_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master-autoscaler" < /dev/null
    elif [[ $NODE == $INGRESS_NODE_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master-ingress" < /dev/null
    elif [[ $NODE == $WORKER_NODE_1_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    elif [[ $NODE == $WORKER_NODE_2_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    elif [[ $NODE == $WORKER_NODE_3_NAME ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    else
      echo "Unknown node type"
    fi
  done
}