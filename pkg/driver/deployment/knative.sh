CONFIG_FILE=$1
export FUNC_NAME=$2

export CPU_REQUEST=$3
export CPU_LIMITS=$4
export MEMORY_REQUESTS=$5
INIT_SCALE=$6
MAX_SCALE=$7

export PANIC_WINDOW=$8
export PANIC_THRESHOLD=$9

export AUTOSCALING_METRIC=${10}
export AUTOSCALING_TARGET=${11}

export COLD_START_BUSY_LOOP_MS=${12}

cat $CONFIG_FILE | envsubst | kn service apply $FUNC_NAME --scale-init $INIT_SCALE --scale-max $MAX_SCALE --wait-timeout 2000000 -f /dev/stdin
