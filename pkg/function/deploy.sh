#!/usr/bin/env bash
CONFIG_FILE=$1
export FUNC_NAME=$2

export MEMORY_REQUEST=$3
export CPU_REQUEST=$4
INIT_SCALE=$5

export PANIC_WINDOW=$6
export PANIC_THRESHOLD=$7

cat $CONFIG_FILE | envsubst | kn service apply $FUNC_NAME --scale-init $INIT_SCALE --concurrency-target 1 --wait-timeout 240 -f /dev/stdin

