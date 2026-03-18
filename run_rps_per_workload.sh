#!/bin/bash


# kubectl set env deployment/activator -n knative-serving NO_SCALEDOWN=true
# kubectl rollout restart -n knative-serving deployment/activator
# kubectl rollout restart -n knative-serving deployment/autoscaler
# sleep 10
# go run experiment/khala_command.go --command=set-corepool --corepool-node="10.0.1.3" --corepool-size="IO:8@1.0,C:20@2.2"

workload_list=(chameleonserve cnnserve imageresize lrserving mapper pyaesserve reducer rnnserve streducer sttrainer)
# workload_list=(rnnserve imageresize)

for workload in ${workload_list[@]}; do
    for max_multiplier in 20
    do
        divisor=10
        WARMUP_SCALE=1
        EXPWARMUP=1
        START_SCALE=1
        END_SCALE=$max_multiplier
        STEP=1
        EXP_DUR=$(((END_SCALE - START_SCALE) / STEP + 1))
        PREFETCH=false

        ### baseline
        EXP="baseline_w-${workload}_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
        echo "Running experiment $EXP"

        python3 generate_scaled_trace.py \
            --divisor $divisor \
            --start-scale $START_SCALE \
            --end-scale $END_SCALE \
            --step $STEP \
            --warmup-duration $EXPWARMUP \
            --warmup-scale $WARMUP_SCALE \
            --single-workload $workload

        # mkdir -p data/out/$EXP
        # go run experiment/khala_command.go --command=deploy
        # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
        # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
        # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
        # go run experiment/khala_command.go --command=clean --remove-snapshots=false

        # sleep 60


        # ### logical sep with prefetch
        # EXP="logicalsep_w-${workload}_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_p-${PREFETCH}"
        # echo "Running experiment $EXP"
        # python3 generate_scaled_trace.py \
        #     --divisor $divisor \
        #     --start-scale $START_SCALE \
        #     --end-scale $END_SCALE \
        #     --step $STEP \
        #     --warmup-duration $EXPWARMUP \
        #     --warmup-scale $WARMUP_SCALE --s3 --rpc \
        #     --single-workload $workload
        
        # mkdir -p data/out/$EXP
        # go run experiment/khala_command.go --command=deploy --set-nexus-sdk=true --set-nexus-rpc=true
        # cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
        # go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
        # kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
        # go run experiment/khala_command.go --command=clean --remove-snapshots=false

        # sleep 60
    done
done