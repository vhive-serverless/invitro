#!/bin/bash
# This script safely adds or updates the kubelet config file
# to reserve CPUs and set the CPU manager policy.

set -e

KUBELET_CONFIG="/var/lib/kubelet/config.yaml"
RESERVED_CPUS='reservedSystemCPUs: "2-27"'
CPU_POLICY='cpuManagerPolicy: "None"'

# --- Safety Check ---
if [ ! -f "$KUBELET_CONFIG" ]; then
    echo "Error: Kubelet config file not found at $KUBELET_CONFIG"
    echo "Cannot proceed. Please ensure the file exists."
    exit 1
fi

echo "Backing up $KUBELET_CONFIG to $KUBELET_CONFIG.bak..."
sudo cp "$KUBELET_CONFIG" "$KUBELET_CONFIG.bak"

echo "Updating $KUBELET_CONFIG..."

# --- Make Changes Idempotently ---
# 1. Remove existing lines to prevent duplicates
sudo sed -i '/^reservedSystemCPUs:/d' "$KUBELET_CONFIG"
sudo sed -i '/^cpuManagerPolicy:/d' "$KUBELET_CONFIG"

# 2. Append the new, correct lines (using the style you requested)
echo "$RESERVED_CPUS" | sudo tee -a "$KUBELET_CONFIG" > /dev/null
echo "$CPU_POLICY" | sudo tee -a "$KUBELET_CONFIG" > /dev/null

echo "Successfully updated $KUBELET_CONFIG."
echo ""

echo "Restarting kubelet to apply changes..."
sudo systemctl restart kubelet

echo "Kubelet restarted. Please verify the changes in $KUBELET_CONFIG."