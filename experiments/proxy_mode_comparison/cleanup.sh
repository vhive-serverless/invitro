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

echo "[$(date +%T)] Deleting deployment and service..."
kubectl delete deployment massive-scale-deployment --ignore-not-found=true
kubectl delete service massive-scale-service --ignore-not-found=true

echo "[$(date +%T)] Waiting for pods to terminate gracefully (up to 60s)..."
kubectl wait --for=delete pod -l app=fake-workload --timeout=60s 2>/dev/null || true

# Check for stuck pods and force delete
remaining_pods=$(kubectl get pods -l app=fake-workload --no-headers 2>/dev/null | wc -l || echo "0")
if [[ "$remaining_pods" -gt 0 ]]; then
    echo "WARNING: $remaining_pods pods still exist. Forcing deletion..."
    kubectl delete pods -l app=fake-workload --force --grace-period=0 2>/dev/null || true
    
    echo "[$(date +%T)] Waiting for forced deletion to complete..."
    sleep 5
fi

# Final verification
final_pods=$(kubectl get pods -l app=fake-workload --no-headers 2>/dev/null | wc -l || echo "0")
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
