#!/usr/bin/env bash

DUR=$1
NODES=$2

cgexec -g cpuset,memory:loader-cg \
    python3 scripts/experiments/feed_prior_works.py $DUR $NODES