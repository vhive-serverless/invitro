#!/bin/bash
# Stop execution if any command fails or if a pipe fails
set -euo pipefail

echo "Checking KWOK node configuration..."
echo ""

echo "=== Nodes with 'type=kwok' label ==="
# Using || true to prevent the script from exiting if no nodes are found (due to set -e)
kubectl get nodes -l type=kwok -o wide || true

echo ""
echo "=== All KWOK-related nodes (by name pattern) ==="
# Using awk to print the header row (NR==1) OR any row matching kwok/fake case-insensitively
kubectl get nodes | awk 'NR==1 || /[Kk][Ww][Oo][Kk]|[Ff][Aa][Kk][Ee]/' || true

echo ""
echo "=== Sample node labels (first KWOK node found) ==="
# Temporarily disable exit-on-error for the jsonpath query in case it returns empty
set +e
FIRST_NODE=$(kubectl get nodes -l type=kwok -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
set -e

if [[ -n "$FIRST_NODE" ]]; then
    echo "Node: $FIRST_NODE"
    kubectl get node "$FIRST_NODE" --show-labels
else
    echo "No nodes found with label 'type=kwok'"
    echo ""
    echo "Checking for nodes with 'node-fake' pattern:"
    
    set +e
    FIRST_FAKE=$(kubectl get nodes -o name | grep -i fake | head -1 | cut -d/ -f2)
    set -e
    
    if [[ -n "$FIRST_FAKE" ]]; then
        echo "Node: $FIRST_FAKE"
        kubectl get node "$FIRST_FAKE" --show-labels
    else
        echo "No 'fake' nodes found either."
    fi
fi