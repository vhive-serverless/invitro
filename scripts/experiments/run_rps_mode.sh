#!/usr/bin/env bash
START=$1
END=$2
STEP=$3
SLOT=$4
FUNC=$5

cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start $START -end $END -step $STEP -slot $SLOT -totalFunctions $FUNC -server trace" run 2>&1 | tee stress.log