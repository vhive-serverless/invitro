CONFIG_FILE=$1
export FUNC_NAME=$2

export CPU_REQUEST=$3
export CPU_LIMITS=$4
export MEMORY_REQUESTS=$5

export INIT_SCALE=$6
export MAX_SCALE=$7
export MIN_SCALE=$8

export PANIC_WINDOW=$9
export PANIC_THRESHOLD=${10}

export AUTOSCALING_METRIC=${11}
export AUTOSCALING_TARGET=${12}

export COLD_START_BUSY_LOOP_MS=${13}

cat $CONFIG_FILE | envsubst | kn service apply $FUNC_NAME  --wait-timeout 2000000 -f /dev/stdin
