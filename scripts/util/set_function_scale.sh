#!/usr/bin/env bash
SCALE=$1

bash ./pkg/function/deploy.sh \
    workloads/container/trace_func_go.yaml \
    myfunc \
    100Gi \
    1000m \
    $SCALE \
    '"10.0"' \
    '"200.0"'
