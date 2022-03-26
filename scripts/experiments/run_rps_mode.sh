#!/usr/bin/env bash
make ARGS='-mode stress -start 1 -step 1 -slot 120 -server trace' run 2>&1 | tee stress.log