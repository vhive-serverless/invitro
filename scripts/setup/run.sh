#!/bin/bash

run_experiment() {
    EXP=$1
    MASTER_NODE=$2
    func=$3
    delay=$4

    export EXPERIMENT="$EXP"_$func_$delay
    export FUNC=$func
    start_time=`date --rfc-3339=seconds | sed 's/ /T/'`

    mkdir data/out/$EXPERIMENT
    cat cmd/config_${EXP}.json | envsubst '$EXPERIMENT','$FUNC' > cmd/config_tmp.json
    go run cmd/loader.go -config cmd/config_tmp.json -verbosity debug | tee data/out/$EXPERIMENT/loader.log

    mkdir data/out/$EXPERIMENT/autoscaler/; scp $MASTER_NODE:/var/log/pods/knative-serving_autoscaler-*/autoscaler/* data/out/$EXPERIMENT/autoscaler/
    sleep 60
    ssh $MASTER_NODE curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot
    mkdir data/out/$EXPERIMENT/prometheus_snapshot
    kubectl cp -n monitoring prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/ -c prometheus data/out/$EXPERIMENT/prometheus_snapshot

    make clean
}

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":10000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "10"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":5000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "5"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":9000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "9"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":3000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "3"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":1000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "1"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":20000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "20"

kubectl patch stage pod-ready -p '{"spec":{"delay":{"durationMilliseconds":7000}}}' --type=merge
run_experiment "kwok" "hancheng@pc855.emulab.net" "400" "7"