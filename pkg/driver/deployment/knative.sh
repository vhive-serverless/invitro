CONFIG_FILE=$1
export FUNC_NAME=$2

export CPU_REQUEST=$3
export CPU_LIMITS=$4
export MEMORY_REQUESTS=$5
INIT_SCALE=$6

export PANIC_WINDOW=$7
export PANIC_THRESHOLD=$8

export AUTOSCALING_METRIC=$9
export AUTOSCALING_TARGET=${10}

export COLD_START_BUSY_LOOP_MS=${11}

cat $CONFIG_FILE | envsubst | kn service apply $FUNC_NAME --scale-init $INIT_SCALE --wait-timeout 2000000 -f /dev/stdin
