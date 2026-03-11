#!/bin/bash
################################################################################
# Cleanup Script for Kube-Proxy Experiment
# 
# This script forcibly cleans up resources created during the experiment,
# useful if the experiment was interrupted or failed.
################################################################################

set -euo pipefail

WAIT_TIME=${1:-10}  # Optional wait time argument, defaults to 10s

echo "========================================"
echo "Cleaning up experiment resources..."
echo "========================================"

echo "[$(date +%T)] Deleting deployment and service (Async Mode)..."
kubectl delete deployment massive-scale-deployment --ignore-not-found=true --wait=false 2>/dev/null || true
kubectl get deployments -o name | grep massive-delta | xargs -r kubectl delete --ignore-not-found=true --wait=false 2>/dev/null || true
kubectl delete service massive-scale-service --ignore-not-found=true --wait=false 2>/dev/null || true

echo "[$(date +%T)] Polling for pod termination status..."
max_checks=30 # 150 seconds max wait (30 * 5s)
i=0
while [ $i -lt $max_checks ]; do
    remaining_pods_old=$(kubectl get pods -l app=fake-workload -o name 2>/dev/null | wc -l || echo "0")
    remaining_pods_new=$(kubectl get pods -l 'delta-id' -o name 2>/dev/null | wc -l || echo "0")
    remaining_pods=$((remaining_pods_old + remaining_pods_new))
    if [ "$remaining_pods" -eq 0 ]; then
        break
    fi
    echo "[$(date +%T)] $remaining_pods pods still terminating..."
    sleep 5
    i=$((i + 1))
done

# Check for stuck pods and force delete
if [[ "$remaining_pods" -gt 0 ]]; then
    echo "WARNING: $remaining_pods pods still stuck. Forcing deletion in background..."
    nohup kubectl delete pods -l app=fake-workload --force --grace-period=0 >/dev/null 2>&1 &
    nohup kubectl delete pods -l 'delta-id' --force --grace-period=0 >/dev/null 2>&1 &
    
    echo "[$(date +%T)] Waiting briefly for background forced deletion to initiate..."
    sleep 5
fi

# Final verification
final_pods_old=$(kubectl get pods -l app=fake-workload --no-headers 2>/dev/null | wc -l || echo "0")
final_pods_new=$(kubectl get pods -l 'delta-id' --no-headers 2>/dev/null | wc -l || echo "0")
final_pods=$((final_pods_old + final_pods_new))
if [[ "$final_pods" -gt 0 ]]; then
    echo "❌ ERROR: $final_pods pods still remain! You may need to investigate manually."
else
    echo "✅ All workload pods terminated."
fi

# Clean up endpointslices just in case
echo "[$(date +%T)] Cleaning up orphaned EndpointSlices..."
kubectl delete endpointslices -l kubernetes.io/service-name=massive-scale-service --ignore-not-found=true 2>/dev/null || true

if [[ "$WAIT_TIME" -gt 0 ]]; then
    echo "[$(date +%T)] Waiting ${WAIT_TIME}s for network rules to settle..."
    sleep "$WAIT_TIME"
fi

echo "========================================"
echo "Cleanup complete!"
echo "========================================"
