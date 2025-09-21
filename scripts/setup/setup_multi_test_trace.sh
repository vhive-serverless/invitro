#!/bin/bash

# Duplicate example traces to make multi traces
mkdir -p data/multi_traces/
cp -r data/traces/example data/multi_traces/example_1_test
cp -r data/traces/example data/multi_traces/example_2_test
cp -r data/traces/example data/multi_traces/example_3.1_test
