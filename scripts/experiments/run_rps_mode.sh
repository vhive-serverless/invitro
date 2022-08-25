#!/usr/bin/env bash
START=$1
END=$2
STEP=$3
SLOT=$4
FUNC=$5
SERVER=$6
RUNTIME=$7
MEMORY=$8
LOG=$9
IAT=${10}

# Example:
# ./scripts/experiments/run_rps_mode.sh 1 20 1 60 1 trace 1000 170 all equidistance
cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start $START -end $END -step $STEP -slot $SLOT -totalFunctions $FUNC -server $SERVER -funcDuration $RUNTIME -funcMemory $MEMORY -print $LOG -iatDistribution $IAT" run 2>&1 | tee stress.log