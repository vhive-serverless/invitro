#!/usr/bin/env bash
START=$1
END=$2
STEP=$3
SLOT=$4
FUNC=$5
SERVER=$6

# ./scripts/experiments/run_rps_mode.sh 1 209 1 30 1 trace
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start $START -end $END -step $STEP -slot $SLOT -totalFunctions $FUNC -server $SERVER" run 2>&1 | tee stress.log