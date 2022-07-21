#!/usr/bin/env bash
SCLAE=$1

bash ./pkg/function/deploy.sh \
    workloads/container/trace_func_go.yaml \
    myfunc \
    100Gi \
    1000m \
    $SCLAE \
    '"10.0"' \
    '"200.0"'

# kn service apply myfunc -f workloads/container/trace_func_go.yaml --scale-min $SCLAE --wait-timeout 2000000