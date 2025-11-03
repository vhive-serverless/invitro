#!/bin/bash


# kubectl set env deployment/activator -n knative-serving NO_SCALEDOWN=true
# kubectl rollout restart -n knative-serving deployment/activator
# sleep 10

for max_multiplier in 20
do
    divisor=100
    EXPWARMUP=1
    EXP_DUR=$max_multiplier

    # test baseline, logical sep, physical sep, dynamic core pool

    ### baseline
    echo "Running Baseline with function multiplier: $max_multiplier"
    EXP="baseline_m-${max_multiplier}_d-${divisor}"

    python3 generate_scaled_trace.py \
        --divisor $divisor \
        --start-scale 1 \
        --end-scale $max_multiplier \
        --step 1 \
        --warmup-duration $EXPWARMUP \
        --warmup-scale 1 \
        --max-scale 25

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 60


    ### logical sep
    echo "Running Logical Sep with function multiplier: $max_multiplier"
    EXP="logicalsep_m-${max_multiplier}_d-${divisor}"
    python3 generate_scaled_trace.py \
        --divisor $divisor \
        --start-scale 1 \
        --end-scale $max_multiplier \
        --step 1 \
        --warmup-duration $EXPWARMUP \
        --warmup-scale 1 \
        --max-scale 25 --s3 --rpc
    
    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 60


    ### physical sep
    echo "Running Physical Sep with function multiplier: $max_multiplier"
    EXP="physicalsep_m-${max_multiplier}_d-${divisor}"
    python3 generate_scaled_trace.py \
        --divisor $divisor \
        --start-scale 1 \
        --end-scale $max_multiplier \
        --step 1 \
        --warmup-duration $EXPWARMUP \
        --warmup-scale 3 \
        --max-scale 25 --s3 --rpc

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_static
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 60


    ### dynamic frequency scaling
    echo "Running Dynamic Frequency Scaling with function multiplier: $max_multiplier"
    EXP="dynamicfreq_m-${max_multiplier}_d-${divisor}"
    python3 generate_scaled_trace.py \
        --divisor $divisor \
        --start-scale 1 \
        --end-scale $max_multiplier \
        --step 1 \
        --warmup-duration $EXPWARMUP \
        --warmup-scale 3 \
        --max-scale 25 --s3 --rpc

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_dynamic
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    sleep 60

done
