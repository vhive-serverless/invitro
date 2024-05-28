#!/bin/bash

run_experiment() {
    EXP=$1
    MASTER_NODE=$2
    func=$3
    AUTOSCALER_NODE=$4
    ACTIVATOR_NODE=$5

    export EXPERIMENT="$EXP"_$func
    export FUNC=$func
    start_time=`date --rfc-3339=seconds | sed 's/ /T/'`

    mkdir data/out/$EXPERIMENT
    cat cmd/config_${EXP}.json | envsubst '$EXPERIMENT','$FUNC' > cmd/config_tmp.json
    go run cmd/loader.go -config cmd/config_tmp.json -verbosity debug | tee data/out/$EXPERIMENT/loader.log

    mkdir data/out/$EXPERIMENT/autoscaler/; scp $AUTOSCALER_NODE:/var/log/pods/knative-serving_autoscaler-*/autoscaler/* data/out/$EXPERIMENT/autoscaler/
    mkdir data/out/$EXPERIMENT/activator/; scp $ACTIVATOR_NODE:/var/log/pods/knative-serving_activator-*/activator/* data/out/$EXPERIMENT/activator/
    
    i=10
    while [ "$(ssh $MASTER_NODE 'curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot | jq ".status"')" != '"success"' ] && [ $i -gt 0 ]; do
            echo retry
            let i=$i-1
            sleep 60
    done

    mkdir data/out/$EXPERIMENT/prometheus_snapshot
    kubectl cp -n monitoring prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/ -c prometheus data/out/$EXPERIMENT/prometheus_snapshot

    make clean
}

kubectl patch deployment istio-ingressgateway -n istio-system --patch '{"spec":{"template":{"spec":{"containers":[{"name":"istio-proxy","resources":{"limits":{"cpu":"10"}}}]}}}}'

kubectl patch deployment istio-ingressgateway -n istio-system --patch '{"spec":{"template":{"spec":{"containers":[{"name":"istio-proxy","resources":{"limits":{"memory":"20Gi"}}}]}}}}'

kubectl patch deployment coredns -n kube-system --patch '{"spec":{"template":{"spec":{"containers":[{"name":"coredns","resources":{"limits":{"memory":"20Gi"}}}]}}}}'

kubectl patch deployment cluster-local-gateway -n istio-system --patch '{"spec":{"template":{"spec":{"containers":[{"name":"istio-proxy","resources":{"limits":{"memory":"20Gi"}}}]}}}}'

kubectl patch deploy -n istio-system istio-ingressgateway -p '{"spec": {"template": {"spec": {"nodeSelector": {"loader-nodetype": "master-ingress"}}}}}'

kubectl patch deployment autoscaler -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "autoscaler", "image": "lkondras/autoscaler-12c0fa24db31956a7cfa673210e4fa13:synchronous-kwok"}]}}}}'

kubectl patch deployment activator -n knative-serving -p '{"spec": {"template": {"spec": {"containers": [{"name": "activator", "image": "lkondras/activator-ecd51ca5034883acbe737fde417a3d86:synchronous-kwok"}]}}}}'

kubectl patch configmap config-autoscaler -n knative-serving -p '{"data": {"container-concurrency-target-percentage": "100"}}'

kubectl patch deployment autoscaler -n knative-serving --patch '{"spec": {"template": {"spec": {"containers": [{"name": "autoscaler", "env": [{"name": "KUBE_API_BURST", "value": "2000"}, {"name": "KUBE_API_QPS", "value": "1000"}]}]}}}}'

# kwok experiment
run_experiment "kwok" "hancheng@pc826.emulab.net" "400" "hancheng@pc751.emulab.net" "hancheng@pc827.emulab.net"
run_experiment "kwok" "hancheng@pc826.emulab.net" "500" "hancheng@pc751.emulab.net" "hancheng@pc827.emulab.net"
run_experiment "kwok" "hancheng@pc826.emulab.net" "1000" "hancheng@pc751.emulab.net" "hancheng@pc827.emulab.net"
run_experiment "kwok" "hancheng@pc826.emulab.net" "2500" "hancheng@pc751.emulab.net" "hancheng@pc827.emulab.net"
run_experiment "kwok" "hancheng@pc826.emulab.net" "2000" "hancheng@pc751.emulab.net" "hancheng@pc827.emulab.net"