#!/bin/bash

run_experiment() {
    EXP=$1
    MASTER_NODE=$2
    func=$3

    export EXPERIMENT="$EXP"_$func
    export FUNC=$func
    start_time=`date --rfc-3339=seconds | sed 's/ /T/'`

    mkdir data/out/$EXPERIMENT
    cat cmd/config_${EXP}.json | envsubst '$EXPERIMENT','$FUNC' > cmd/config_tmp.json
    go run cmd/loader.go -config cmd/config_tmp.json -verbosity debug | tee data/out/$EXPERIMENT/loader.log

    mkdir data/out/$EXPERIMENT/autoscaler/; scp $MASTER_NODE:/var/log/pods/knative-serving_autoscaler-*/autoscaler/* data/out/$EXPERIMENT/autoscaler/
    ssh $MASTER_NODE curl -XPOST http://localhost:9090/api/v1/admin/tsdb/snapshot
    mkdir data/out/$EXPERIMENT/prometheus_snapshot
    kubectl cp -n monitoring prometheus-prometheus-kube-prometheus-prometheus-0:/prometheus/snapshots/ -c prometheus data/out/$EXPERIMENT/prometheus_snapshot

    make clean
}

# baseline experiment
run_experiment "baseline" "hancheng@pc739.emulab.net" "400"

# # kwok experiment
# run_experiment "kwok" "hancheng@pc739.emulab.net" "400"

# # kwok experiment
# run_experiment "kwok" "hancheng@pc739.emulab.net" "1000"