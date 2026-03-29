#!/bin/bash


# kubectl set env deployment/activator -n knative-serving NO_SCALEDOWN=true
# kubectl rollout restart -n knative-serving deployment/activator
# kubectl rollout restart -n knative-serving deployment/autoscaler
# sleep 10
# go run experiment/khala_command.go --command=set-corepool --corepool-node="10.0.1.3" --corepool-size="IO:8@1.0,C:20@2.2"


# for max_multiplier in 29
for max_multiplier in 20
do
    divisor=50
    EXPWARMUP=5
    START_SCALE=5
    END_SCALE=$max_multiplier
    STEP=5
    EXP_DUR=$(((END_SCALE - START_SCALE) / STEP + 1))
    PREFETCH=false

    # test baseline, sdk only, nexus only, nexus + prefetch

    # ### sdk only
    # echo "Running SDK Only with function multiplier: $max_multiplier"
    # EXP="sdkonly_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
    # python3 generate_trace_sweep.py \
    #     --divisor $divisor \
    #     --start-scale $START_SCALE \
    #     --end-scale $END_SCALE \
    #     --step $STEP \
    #     --shift-step 10 \
    #     --warmup-duration $EXPWARMUP \
    #     --warmup-scale 1 --s3
    
    # mkdir -p data/out/$EXP
    # go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true
    # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
    # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    # go run experiment/khala_command.go --command=clean --remove-snapshots=false

    # sleep 120

    # ### Nexus only
    # echo "Running Nexus with function multiplier: $max_multiplier"
    # EXP="nexus_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
    # python3 generate_trace_sweep.py \
    #     --divisor $divisor \
    #     --start-scale $START_SCALE \
    #     --end-scale $END_SCALE \
    #     --step $STEP \
    #     --shift-step 10 \
    #     --warmup-duration $EXPWARMUP \
    #     --warmup-scale 1 --s3 --rpc
    
    # mkdir -p data/out/$EXP
    # go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true --set-nexus-rpc=true
    # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
    # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    # go run experiment/khala_command.go --command=clean --remove-snapshots=false

    # sleep 120
    

    # ### Nexus + Prefetch
    # echo "Running Nexus + Prefetch with function multiplier: $max_multiplier"
    # PREFETCH=true
    # EXP="nexusprefetch_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
    # python3 generate_trace_sweep.py \
    #     --divisor $divisor \
    #     --start-scale $START_SCALE \
    #     --end-scale $END_SCALE \
    #     --step $STEP \
    #     --shift-step 10 \
    #     --warmup-duration $EXPWARMUP \
    #     --warmup-scale 1 --s3 --rpc
    
    # mkdir -p data/out/$EXP
    # go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true --set-nexus-rpc=true
    # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
    # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    # go run experiment/khala_command.go --command=clean --remove-snapshots=false

    # sleep 120

    # ### RDMA
    # echo "Running Nexus + RDMA with function multiplier: $max_multiplier"
    # PREFETCH=false
    # EXP="nexusrdma_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
    # python3 generate_trace_sweep.py \
    #     --divisor $divisor \
    #     --start-scale $START_SCALE \
    #     --end-scale $END_SCALE \
    #     --step $STEP \
    #     --shift-step 10 \
    #     --warmup-duration $EXPWARMUP \
    #     --warmup-scale 1 --s3 --rpc
    
    # mkdir -p data/out/$EXP
    # go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true --set-nexus-rpc=true --with-rdma=true
    # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
    # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    # go run experiment/khala_command.go --command=clean --remove-snapshots=false

    # sleep 120
done

# go run experiment/khala_command.go --command=clean --remove-snapshots=false
# sleep 120

# for max_multiplier in 29
for max_multiplier in 60
do
    divisor=50
    EXPWARMUP=5
    START_SCALE=3
    END_SCALE=$max_multiplier
    STEP=3
    EXP_DUR=$(((END_SCALE - START_SCALE) / STEP + 1))
    PREFETCH=false

    # test baseline, sdk only, nexus only, nexus + prefetch

    ### baseline
    echo "Running Baseline with function multiplier: $max_multiplier, durations: $EXP_DUR"
    EXP="baseline_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
    echo "Experiment: $EXP"

    python3 generate_trace_sweep.py \
        --divisor $divisor \
        --start-scale $START_SCALE \
        --end-scale $END_SCALE \
        --step $STEP \
        --shift-step 10 \
        --warmup-duration $EXPWARMUP \
        --warmup-scale 1

    mkdir -p data/out/$EXP
    go run experiment/khala_command.go --command=deploy
    cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
    go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
    kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
    go run experiment/khala_command.go --command=clean --remove-snapshots=false

    # sleep 120

done
