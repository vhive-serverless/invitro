#!/usr/bin/env bash

DUR=$1
NODES=$2
SERVER=$3

cgexec -g cpuset,memory:loader-cg \
    python3 scripts/experiments/drive_trace_mode.py $DUR $NODES $SERVER