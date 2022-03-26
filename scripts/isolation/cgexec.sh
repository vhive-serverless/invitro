#!/usr/bin/env bash

cgexec -g cpuset,memory:loader-cg $*

# Just in case.
cgclassify -g cpuset,memory:loader-cg $(pidof load)
taskset -cp 12-15 $(pidof load)
