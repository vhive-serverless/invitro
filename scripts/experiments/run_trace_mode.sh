#!/usr/bin/env bash

DUR=$1
NODES=$2

cgexec -g cpuset,memory:loader-cg \
    python3 scripts/experiments/baseline_capacity.py $DUR $NODES