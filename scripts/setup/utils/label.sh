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
# label_master $MASTER_NODE - label master node as master
# label_loader $LOADER_NODE - label loader node as monitoring
# label_workers $WORKER_NODE - label worker node as worker
# label_mocks3 $MASTER_NODE - label mocks3 node as mocks3
######################################################

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $1 $2;
}

label_nodes() {
  MASTER_NODE=$1
  LOADER_NODE=$2
  WORKER_NODE_0=$3
  WORKER_NODE_1=$4
  MASTER_NODE_NAME="$(server_exec "$MASTER_NODE" hostname)"
  LOADER_NODE_NAME="$(server_exec "$LOADER_NODE" hostname)"
  WORKER_NODE_NAME_0="$(server_exec "$WORKER_NODE_0" hostname)"
  WORKER_NODE_NAME_1="$(server_exec "$WORKER_NODE_1" hostname)"


  echo $LOADER_NODE_NAME
  echo $WORKER_NODE_NAME

  server_exec $MASTER_NODE 'kubectl get nodes' | tail +2 | while IFS= read -r LINE
  do
    NODE=$(echo $LINE | cut -d ' ' -f 1)
    TYPE=$(echo $LINE | cut -d ' ' -f 3)

    echo "Label ${NODE}"
    if [[ $TYPE == *"control-plane"* ]]; then
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=master" < /dev/null
      echo "Label ${NODE} as minio-operator"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} minio-type=operator" < /dev/null
    elif [[ $NODE == $LOADER_NODE_NAME ]]; then
      echo "Label ${NODE} as loader"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=monitoring" < /dev/null
    elif [[ $NODE == $WORKER_NODE_NAME_0 ]]; then
      echo "Label ${NODE} as worker and IO-Intensive Node"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    elif [[ $NODE == $WORKER_NODE_NAME_1 ]]; then
      echo "Label ${NODE} as worker and non-IO_Intensive Node"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} loader-nodetype=worker" < /dev/null
    else
      echo "Label ${NODE} as minio-tenant"
      server_exec $MASTER_NODE "kubectl label nodes ${NODE} minio-type=tenant" < /dev/null
    fi
  done
}
