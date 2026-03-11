#!/bin/bash

################################################################################
# Kubernetes Control Plane Turbocharger
# 
# This script increases the rate limits (QPS/Burst) of the Kubernetes control
# plane components and kube-proxy to allow for massive, rapid scale-ups.
################################################################################

set -e

# Target speed limits (Moderate bump to protect etcd)
QPS=100
API_MUTATING_INFLIGHT=500
API_MAX_INFLIGHT=1000

MANIFEST_DIR="/etc/kubernetes/manifests"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="${MANIFEST_DIR}/backup_${TIMESTAMP}"

echo "=================================================="
echo "🚀 Turbocharging Kubernetes Control Plane..."
echo "=================================================="

# 1. Backup existing manifests
echo "[1/5] Creating backups in ${BACKUP_DIR}..."
sudo mkdir -p "${BACKUP_DIR}"
sudo cp ${MANIFEST_DIR}/*.yaml "${BACKUP_DIR}/"

# Function to safely inject or update a flag
inject_or_update_flag() {
    local file=$1
    local cmd=$2
    local flag=$3
    
    # Extract the base flag name (e.g., "--kube-api-qps")
    local flag_base=$(echo "$flag" | cut -d= -f1)
    
    if sudo grep -q "${flag_base}=" "$file"; then
        # Flag exists, update its value
        sudo sed -i "s|${flag_base}=.*|${flag}|g" "$file"
        echo "      Updated: ${flag}"
    else
        # Flag does not exist, append it directly under the binary command
        sudo sed -i "s|- ${cmd}$|- ${cmd}\n    - ${flag}|" "$file"
        echo "      Inserted: ${flag}"
    fi
}

# 2. Modify Kube-Apiserver
echo "[2/5] Patching kube-apiserver..."
API_FILE="${MANIFEST_DIR}/kube-apiserver.yaml"
inject_or_update_flag "$API_FILE" "kube-apiserver" "--max-mutating-requests-inflight=${API_MUTATING_INFLIGHT}"
inject_or_update_flag "$API_FILE" "kube-apiserver" "--max-requests-inflight=${API_MAX_INFLIGHT}"

# 3. Modify Kube-Controller-Manager
echo "[3/5] Patching kube-controller-manager..."
CM_FILE="${MANIFEST_DIR}/kube-controller-manager.yaml"
inject_or_update_flag "$CM_FILE" "kube-controller-manager" "--kube-api-qps=${QPS}"
inject_or_update_flag "$CM_FILE" "kube-controller-manager" "--concurrent-deployment-syncs=20"
inject_or_update_flag "$CM_FILE" "kube-controller-manager" "--concurrent-replicaset-syncs=20"
inject_or_update_flag "$CM_FILE" "kube-controller-manager" "--concurrent-endpointslice-syncs=50"

# 4. Modify Kube-Scheduler
echo "[4/5] Patching kube-scheduler..."
SCHED_FILE="${MANIFEST_DIR}/kube-scheduler.yaml"
inject_or_update_flag "$SCHED_FILE" "kube-scheduler" "--kube-api-qps=${QPS}"

# 5. Modify Kube-Proxy ConfigMap
echo "[5/5] Patching kube-proxy ConfigMap..."
# Export the configmap, modify the client limits, and apply it
kubectl get configmap kube-proxy -n kube-system -o yaml > /tmp/kube-proxy-cm.yaml

# Only update if the fields exist, otherwise append them
if grep -q "qps:" /tmp/kube-proxy-cm.yaml; then
    sed -i "s/qps: .*/qps: ${QPS}/g" /tmp/kube-proxy-cm.yaml
else
    # Insert qps after clientConnection: line
    sed -i "/clientConnection:/a\    qps: ${QPS}" /tmp/kube-proxy-cm.yaml
fi

kubectl apply -f /tmp/kube-proxy-cm.yaml
rm /tmp/kube-proxy-cm.yaml

echo ""
echo "=================================================="
echo "✅ Modifications Complete!"
echo "=================================================="
echo "The Kubelet will automatically detect the changes to the manifest files"
echo "and restart the control plane components over the next 30-60 seconds."
echo ""
echo "Restarting kube-proxy daemonset to pick up the new client limits..."
kubectl rollout restart daemonset kube-proxy -n kube-system
echo "Done."
echo ""
echo "⚠️  IMPORTANT: Wait 2-3 minutes for all components to fully restart"
echo "before running experiments to ensure rate limits are active."
echo ""
echo "=================================================="
echo "🔍 Verification Steps"
echo "=================================================="
echo ""
echo "[1/3] Waiting for kube-proxy rollout to complete..."
kubectl rollout status daemonset kube-proxy -n kube-system --timeout=180s

echo ""
echo "[2/3] Verifying control plane pods are running..."
sleep 30  # Give kubelet time to restart static pods

CONTROL_PLANE_HEALTHY=true
for component in kube-apiserver kube-controller-manager kube-scheduler; do
    POD_STATUS=$(kubectl get pods -n kube-system -l component=${component} -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "NotFound")
    if [[ "$POD_STATUS" == "Running" ]]; then
        echo "   ✅ ${component}: Running"
    else
        echo "   ❌ ${component}: ${POD_STATUS}"
        CONTROL_PLANE_HEALTHY=false
    fi
done

echo ""
echo "[3/3] Verifying configuration values..."

# Check API server flags
echo "   Checking kube-apiserver flags..."
if sudo grep -q "max-mutating-requests-inflight=${API_MUTATING_INFLIGHT}" "${MANIFEST_DIR}/kube-apiserver.yaml"; then
    echo "      ✅ max-mutating-requests-inflight: ${API_MUTATING_INFLIGHT}"
else
    echo "      ❌ max-mutating-requests-inflight: NOT SET"
    CONTROL_PLANE_HEALTHY=false
fi

if sudo grep -q "max-requests-inflight=${API_MAX_INFLIGHT}" "${MANIFEST_DIR}/kube-apiserver.yaml"; then
    echo "      ✅ max-requests-inflight: ${API_MAX_INFLIGHT}"
else
    echo "      ❌ max-requests-inflight: NOT SET"
    CONTROL_PLANE_HEALTHY=false
fi

# Check controller-manager flags
echo "   Checking kube-controller-manager flags..."
if sudo grep -q "kube-api-qps=${QPS}" "${MANIFEST_DIR}/kube-controller-manager.yaml"; then
    echo "      ✅ kube-api-qps: ${QPS}"
else
    echo "      ❌ kube-api-qps: NOT SET"
    CONTROL_PLANE_HEALTHY=false
fi

# Check scheduler flags
echo "   Checking kube-scheduler flags..."
if sudo grep -q "kube-api-qps=${QPS}" "${MANIFEST_DIR}/kube-scheduler.yaml"; then
    echo "      ✅ kube-api-qps: ${QPS}"
else
    echo "      ❌ kube-api-qps: NOT SET"
    CONTROL_PLANE_HEALTHY=false
fi

# Check kube-proxy configmap
echo "   Checking kube-proxy ConfigMap..."
PROXY_QPS=$(kubectl get configmap kube-proxy -n kube-system -o jsonpath='{.data.config\.conf}' 2>/dev/null | grep -oP 'qps:\s*\K\d+' || echo "0")
if [[ "$PROXY_QPS" -ge "$QPS" ]]; then
    echo "      ✅ qps: ${PROXY_QPS}"
else
    echo "      ❌ qps: ${PROXY_QPS} (expected ${QPS})"
    CONTROL_PLANE_HEALTHY=false
fi

echo ""
echo "=================================================="
if [[ "$CONTROL_PLANE_HEALTHY" == "true" ]]; then
    echo "✅ All verifications passed! Control plane is turbocharged."
    echo "=================================================="
    exit 0
else
    echo "⚠️  Some verifications failed. Check the output above."
    echo "   You may need to manually verify the configuration."
    echo "=================================================="
    exit 1
fi
