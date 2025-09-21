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

MASTER_NODE=$1

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

echo "Installing trace visualizer"

server_exec 'git clone https://github.com/ease-lab/vSwarm'
VSWARM='vSwarm/tools/trace-plotter/'

server_exec "cd ${VSWARM}; helm repo add openzipkin https://openzipkin.github.io/zipkin"
server_exec "cd ${VSWARM}; helm pull --untar openzipkin/zipkin"
server_exec "cd ${VSWARM}; helm repo add bitnami https://charts.bitnami.com/bitnami"
server_exec "cd ${VSWARM}; helm pull --untar bitnami/elasticsearch"

server_exec "kubectl create namespace elasticsearch"
server_exec "kubectl create namespace zipkin"

server_exec "cd ${VSWARM}; helm upgrade --install --wait -f ./values/es-example.values.yaml -n elasticsearch elasticsearch ./elasticsearch"
server_exec "cd ${VSWARM}; helm upgrade --install --wait -f ./values/zipkin-example.values.yaml -n zipkin zipkin ./zipkin"

server_exec "kubectl patch configmap/config-tracing \
  -n knative-serving \
  --type merge \
  -p '{\"data\":{\"backend\":\"zipkin\",\"zipkin-endpoint\":\"http://zipkin.zipkin.svc.cluster.local:9411/api/v2/spans\",\"debug\":\"true\"}}'"

server_exec 'tmux new -s tp_elasticsearch -d'
server_exec 'tmux send -t tp_elasticsearch "nohup kubectl port-forward --namespace elasticsearch svc/elasticsearch 9200:9200 &" ENTER'
server_exec 'tmux new -s tp_zipkin -d'
server_exec 'tmux send -t tp_zipkin "nohup kubectl port-forward --namespace zipkin deployment/zipkin 9411:9411 &" ENTER'

echo "Finished installing the trace visualizer"