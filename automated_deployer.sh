#!/bin/bash

# To use this script, make sure the config.json file to have the TracePath field as: "data/traces/reference/sampled_150/$FUNC"
# and the OutputPathPrefix field as: "data/out/$EXPERIMENT/experiment"

# Run the script as follows:
# ./automated_deployer.sh <start number of functions> <stop number of functions> <step size> <experiment name>


start=$1
stop=$2
step=$3
EXPERIMENT=$4

counter=$start

while [ $counter -le $stop ]
do
    FUNC=$counter
    export FUNC
    export EXPERIMENT
    if [ ! -d "data/traces/reference/sampled_150/$counter/" ]; then
    # increment the counter and continue
        counter=$((counter + step))
        continue
    fi

    # Read the config.json to find if the vSwarm field is set to true
    vSwarm=$(jq -r '.vSwarm' cmd/config_knative_trace.json)
    if [ $vSwarm == "true" ]; then
        echo "vSwarm is set to true, using vSwarm functions"
        cd tools/mapper/
        python3 mapper.py -t ../../data/traces/reference/sampled_150/$counter/ -p profile.json
        cd ../..
    else
        echo "vSwarm is set to false, using dummy functions"
    fi
    cat cmd/config_knative_trace.json | envsubst '${EXPERIMENT},${FUNC}' > cmd/config_tmp.json
    mkdir -p data/out/$EXPERIMENT
    go run cmd/loader.go -config cmd/config_tmp.json -verbosity debug | tee data/out/$EXPERIMENT/loader_$FUNC.log
    counter=$((counter + step))
done

rm cmd/config_tmp.json

echo "Done with the automation script"