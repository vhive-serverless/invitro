#!/usr/bin/env bash

DUR=$1
NODES=$2
SRC=$3

cgexec -g cpuset,memory:loader-cg \
    python3 scripts/experiments/feed_same_size.py $DUR $NODES $SRC