#!/usr/bin/env bash

CONFIG_FILE=$1
export FUNC_NAME=$2

export CPU_REQUEST=$3
export CPU_LIMITS=$4
export MEMORY_REQUESTS=$5
export GPU_REQUEST=$6
export GPU_LIMITS=$7

INIT_SCALE=$8

export PANIC_WINDOW=$9
export PANIC_THRESHOLD=${10}

export AUTOSCALING_METRIC=${11}
export AUTOSCALING_TARGET=${12}

# echo "run start"
# echo $PANIC_THRESHOLD
echo $@
# exit 
# echo "run here"
# cat $CONFIG_FILE | envsubst | cat 
# exit 
cat $CONFIG_FILE | envsubst | kn service apply $FUNC_NAME --scale-init $INIT_SCALE --concurrency-target 1 --wait-timeout 2000000 -f /dev/stdin


# exit 