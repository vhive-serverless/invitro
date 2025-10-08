#!/bin/bash

for multiplier in 1 2 3 4 5 6 7 8
do
    divisor=100
    WARMUP=$multiplier
    EXP_DUR=5

    # test baseline, logical sep, physical sep, dynamic core pool
    ### baseline
    echo "Running Baseline with function multiplier: $multiplier"
    EXP="baseline_m-${multiplier}_d-${divisor}"
    python3 generate_trace.py \
        --function-multiplier $multiplier \
        --selection-divisor $divisor \
        --warmup $WARMUP

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$WARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 10


    ### logical sep
    echo "Running Logical Sep with function multiplier: $multiplier"
    EXP="logicalsep_m-${multiplier}_d-${divisor}"
    python3 generate_trace.py \
        --function-multiplier $multiplier \
        --selection-divisor $divisor \
        --warmup $WARMUP --s3 --rpc
    
    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy 
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$WARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 10


    ### physical sep
    echo "Running Physical Sep with function multiplier: $multiplier"
    EXP="physicalsep_m-${multiplier}_d-${divisor}"
    python3 generate_trace.py \
        --function-multiplier $multiplier \
        --selection-divisor $divisor \
        --warmup $WARMUP --s3 --rpc

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_static
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$WARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 10


    ### dynamic frequency scaling
    echo "Running Dynamic Frequency Scaling with function multiplier: $multiplier"
    EXP="dynamicfreq_m-${multiplier}_d-${divisor}"
    python3 generate_trace.py \
        --function-multiplier $multiplier \
        --selection-divisor $divisor \
        --warmup $WARMUP --s3 --rpc

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_dynamic
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$WARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 10


done
