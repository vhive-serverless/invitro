#!/usr/bin/env bash
START=$1
STEP=$2
SLOT=$3
FUNC=$4

cgexec -g cpuset,memory:loader-cg \
    make ARGS="-mode stress -start $START -step $STEP -slot $SLOT -totalFunctions $FUNC -server trace" run 2>&1 | tee stress.log