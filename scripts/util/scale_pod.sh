#!/usr/bin/env bash
SCLAE=$1

kn service apply myfunc -f workloads/container/trace_func_go.yaml --scale-min $SCLAE --wait-timeout 2000000