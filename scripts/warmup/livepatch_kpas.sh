#!/usr/bin/env bash
CFG_F=$1
if [ -z "$CFG_F" ]; then
    CFG_F='config/kn_local_patch.yaml'
fi
echo "Patching all podautoscalers using $CFG_F"

readarray -t AUTOSCALERS < <(kubectl -n default get podautoscalers | tail -n +2 | awk '{print $1}')

for autoscaler in ${AUTOSCALERS[@]}; do 
    kubectl -n default patch podautoscaler "$autoscaler" --type=merge --patch-file "$CFG_F"
done