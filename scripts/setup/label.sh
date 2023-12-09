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
  LOADER_NODE_NAME="$(server_exec "$LOADER_NODE" hostname)"
  echo $LOADER_NODE_NAME

  server_exec $MASTER_NODE 'kubectl get nodes' > tmp
  sed -i'' '1d' tmp

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
