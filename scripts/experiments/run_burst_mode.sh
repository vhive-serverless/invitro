#!/usr/bin/env bash
RPS=$1
BURST=$2
DUR=$3
RUNTIME=$4
MEMORY=$5

# ./scripts/experiments/run_burst_mode.sh 8 10 4 500 10 # 16 cores -> 32 max RPS
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode burst -end $RPS -burst $BURST -duration $DUR -funcDuration $RUNTIME -funcMemory $MEMORY" run 2>&1 | tee burst.log