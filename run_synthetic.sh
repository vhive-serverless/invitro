#!/bin/bash


# kubectl set env deployment/activator -n knative-serving NO_SCALEDOWN=true
# kubectl rollout restart -n knative-serving deployment/activator
# kubectl rollout restart -n knative-serving deployment/autoscaler
# sleep 10
# go run experiment/khala_command.go --command=set-corepool --corepool-node="10.0.1.3" --corepool-size="IO:8@1.0,C:20@2.2"


for io_size in 2500000
    do
    for compute_duration in 10
        do
            max_multiplier=60
            divisor=1
            EXPWARMUP=3
            START_SCALE=4
            END_SCALE=$max_multiplier
            STEP=4
            EXP_DUR=$(((END_SCALE - START_SCALE) / STEP + 1))
            PREFETCH=true

            # test baseline, logical sep, physical sep, dynamic core pool

            ### baseline
            echo "Running Baseline with function multiplier: $max_multiplier"
            EXP="baseline_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_c-${compute_duration}_p-${io_size}_r-${PREFETCH}"

            python3 generate_scaled_trace.py \
                --divisor $divisor \
                --start-scale $START_SCALE \
                --end-scale $END_SCALE \
                --step $STEP \
                --warmup-duration $EXPWARMUP \
                --warmup-scale 4 \
                --workload {\"synthetic-c-${compute_duration}-p-${io_size}\":20}

            mkdir -p data/out/$EXP
            go run experiment/khala_command.go --command=deploy
            cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
            go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
            kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
            go run experiment/khala_command.go --command=clean --remove-snapshots=false

            sleep 60

            ### logical sep
            echo "Running Logical Sep with function multiplier: $max_multiplier"
            EXP="logicalsep_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_c-${compute_duration}_p-${io_size}_r-${PREFETCH}"

            python3 generate_scaled_trace.py \
                --divisor $divisor \
                --start-scale $START_SCALE \
                --end-scale $END_SCALE \
                --step $STEP \
                --warmup-duration $EXPWARMUP \
                --warmup-scale 8 --s3 --rpc \
                --workload {\"synthetic-c-${compute_duration}-p-${io_size}\":20} 
            
            mkdir -p data/out/$EXP
            go run experiment/khala_command.go --command=deploy
            cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
            go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
            kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
            go run experiment/khala_command.go --command=clean --remove-snapshots=false

            sleep 60

            ### physical sep
            echo "Running Physical Sep with function multiplier: $max_multiplier"
            EXP="physicalsep_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_c-${compute_duration}_p-${io_size}_r-${PREFETCH}"
            python3 generate_scaled_trace.py \
                --divisor $divisor \
                --start-scale $START_SCALE \
                --end-scale $END_SCALE \
                --step $STEP \
                --warmup-duration $EXPWARMUP \
                --warmup-scale 8 --s3 --rpc \
                --workload {\"synthetic-c-${compute_duration}-p-${io_size}\":20} 

            mkdir -p data/out/$EXP
            go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_static
            cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
            go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
            kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
            go run experiment/khala_command.go --command=clean --remove-snapshots=false

            sleep 60

            ### dynamic frequency scaling
            echo "Running Dynamic Frequency Scaling with function multiplier: $max_multiplier"
            EXP="dynamicfreq_d-${divisor}_s-${START_SCALE}_e-${END_SCALE}_t-${STEP}_c-${compute_duration}_p-${io_size}_r-${PREFETCH}"
            python3 generate_scaled_trace.py \
                --divisor $divisor \
                --start-scale $START_SCALE \
                --end-scale $END_SCALE \
                --step $STEP \
                --warmup-duration $EXPWARMUP \
                --warmup-scale 8 --s3 --rpc \
                --workload {\"synthetic-c-${compute_duration}-p-${io_size}\":20} 

            mkdir -p data/out/$EXP
            go run experiment/khala_command.go --command=deploy --core-pool-policy corepool_freq_dynamic
            cat cmd/config_khala_trace_template.json | EXPERIMENT="$EXP" EXP_DUR="$EXP_DUR" WARMUP="$EXPWARMUP" PREFETCH="$PREFETCH" envsubst > cmd/config_khala_trace.json
            go run cmd/loader.go --config cmd/config_khala_trace.json | tee data/out/$EXP/loader.log
            kubectl logs deployment/activator -n knative-serving > data/out/$EXP/activator.log
            go run experiment/khala_command.go --command=clean --remove-snapshots=false

            sleep 60
    done
done