#!/usr/bin/env bash
make ARGS='-mode stress -start 1 -step 2 -slot 30 -server trace' run 2>&1 | tee stress.log