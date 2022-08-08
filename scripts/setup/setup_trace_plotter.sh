#!/bin/bash

MASTER_NODE=$1

server_exec() {
  ssh -oStrictHostKeyChecking=no -p 22 $MASTER_NODE $1;
}

echo "Installing Dohyun's trace plotter"

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

echo "Finished installing Dohyun's trace plotter"