#!/bin/bash

################################################################################
# Kube-Proxy Mode Comparison Experiment
# 
# This script automates the control plane latency experiment comparing
# iptables vs nftables modes for kube-proxy.
#
# Prerequisites:
# - Prometheus deployed and accessible
# - KWOK fake nodes set up
# - You have already switched kube-proxy mode (iptables/nftables) manually
#
# Usage:
#   ./run_experiment.sh --mode [iptables|nftables] [OPTIONS]
#
################################################################################

set -e

# Default values
MODE=""
DURATION=300
PROMETHEUS_URL="http://localhost:9090"
OUTPUT_DIR="./results"
REPLICAS=5000
CLEANUP=false
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENT_YAML="${SCRIPT_DIR}/massive-scale-deployment.yaml"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode)
            MODE="$2"
            shift 2
            ;;
        --duration)
            DURATION="$2"
            shift 2
            ;;
        --prometheus-url)
            PROMETHEUS_URL="$2"
            shift 2
            ;;
        --output-dir)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        --replicas)
            REPLICAS="$2"
            shift 2
            ;;
        --cleanup)
            CLEANUP=true
            shift
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate required arguments
if [[ -z "$MODE" ]]; then
    echo "Error: --mode is required (iptables or nftables)"
    echo "Usage: $0 --mode [iptables|nftables] [OPTIONS]"
    exit 1
fi

if [[ "$MODE" != "iptables" && "$MODE" != "nftables" ]]; then
    echo "Error: --mode must be either 'iptables' or 'nftables'"
    exit 1
fi

# Create output directory
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULT_DIR="${OUTPUT_DIR}/${MODE}_${TIMESTAMP}"
mkdir -p "$RESULT_DIR"

echo "========================================"
echo "Kube-Proxy Mode Comparison Experiment"
echo "========================================"
echo "Mode:            $MODE"
echo "Replicas:        $REPLICAS"
echo "Duration:        ${DURATION}s"
echo "Prometheus URL:  $PROMETHEUS_URL"
echo "Output Dir:      $RESULT_DIR"
echo "========================================"

# Function to query Prometheus
query_prometheus() {
    local query="$1"
    local output_file="$2"
    local timestamp=$(date +%s)
    
    curl -s -G "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode "query=${query}" \
        --data-urlencode "time=${timestamp}" \
        -o "${output_file}"
}

# Function to query Prometheus range
query_prometheus_range() {
    local query="$1"
    local output_file="$2"
    local start="$3"
    local end="$4"
    local step="${5:-5}"  # Default 5s step
    
    curl -s -G "${PROMETHEUS_URL}/api/v1/query_range" \
        --data-urlencode "query=${query}" \
        --data-urlencode "start=${start}" \
        --data-urlencode "end=${end}" \
        --data-urlencode "step=${step}" \
        -o "${output_file}"
}

# Function to collect baseline metrics
collect_baseline_metrics() {
    echo ""
    echo "[$(date +%T)] Collecting baseline metrics..."
    
    query_prometheus 'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))' \
        "${RESULT_DIR}/baseline_cpu.json"
    
    query_prometheus 'sum(process_resident_memory_bytes{job="kube-proxy"})' \
        "${RESULT_DIR}/baseline_memory.json"
    
    query_prometheus 'count(kube_pod_info)' \
        "${RESULT_DIR}/baseline_pod_count.json"
    
    query_prometheus 'count(kube_service_info)' \
        "${RESULT_DIR}/baseline_service_count.json"
    
    echo "[$(date +%T)] Baseline metrics collected"
}

# Function to verify kube-proxy mode
verify_proxy_mode() {
    echo ""
    echo "[$(date +%T)] Verifying kube-proxy mode..."
    
    ACTUAL_MODE=$(kubectl -n kube-system get cm kube-proxy -o yaml | grep "mode:" | awk '{print $2}' | tr -d '"' || echo "unknown")
    
    echo "Expected mode: $MODE"
    echo "Actual mode:   $ACTUAL_MODE"
    
    if [[ "$ACTUAL_MODE" != "$MODE" ]]; then
        echo ""
        echo "WARNING: kube-proxy mode mismatch!"
        echo "You specified --mode=$MODE but kube-proxy is running in $ACTUAL_MODE mode"
        read -p "Continue anyway? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
    
    echo "$ACTUAL_MODE" > "${RESULT_DIR}/proxy_mode.txt"
}

# Function to update replica count in YAML
prepare_deployment() {
    echo ""
    echo "[$(date +%T)] Preparing deployment with $REPLICAS replicas..."
    
    if [[ ! -f "$DEPLOYMENT_YAML" ]]; then
        echo "Error: Cannot find deployment YAML at $DEPLOYMENT_YAML"
        exit 1
    fi
    
    cp "$DEPLOYMENT_YAML" "${RESULT_DIR}/deployment.yaml"
    sed -i "s/replicas: [0-9]*/replicas: ${REPLICAS}/" "${RESULT_DIR}/deployment.yaml"
}

# Function to apply deployment
apply_deployment() {
    echo ""
    echo "[$(date +%T)] Applying massive-scale deployment..."
    
    DEPLOY_START=$(date +%s)
    echo "$DEPLOY_START" > "${RESULT_DIR}/deploy_start_timestamp.txt"
    
    kubectl apply -f "${RESULT_DIR}/deployment.yaml"
    
    echo "[$(date +%T)] Deployment applied. Monitoring kube-proxy sync metrics..."
}

# Function to monitor deployment progress
monitor_deployment() {
    echo ""
    echo "[$(date +%T)] Monitoring deployment for ${DURATION} seconds..."
    
    local end_time=$(($(date +%s) + DURATION))
    local interval=10
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local pods_ready=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        local pods_total=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.replicas}' 2>/dev/null || echo "0")
        
        # Use endpointslices to bypass the 1000 Endpoints API hard limit
        local endpoints=$(kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o jsonpath='{.items[*].endpoints[*].addresses[*]}' 2>/dev/null | wc -w)
        
        echo "[$(date +%T)] Pods: ${pods_ready}/${pods_total} | Endpoints: ${endpoints}"
        
        sleep $interval
    done
    
    DEPLOY_END=$(date +%s)
    echo "$DEPLOY_END" > "${RESULT_DIR}/deploy_end_timestamp.txt"
    
    echo "[$(date +%T)] Monitoring period complete"
}

# Function to collect post-deployment metrics
collect_deployment_metrics() {
    echo ""
    echo "[$(date +%T)] Collecting deployment metrics..."
    
    local start_time=$(cat "${RESULT_DIR}/deploy_start_timestamp.txt")
    local end_time=$(cat "${RESULT_DIR}/deploy_end_timestamp.txt")
    
    echo "Querying kube-proxy sync duration..."
    query_prometheus_range \
        'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' \
        "${RESULT_DIR}/sync_duration_p99_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    query_prometheus_range \
        'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' \
        "${RESULT_DIR}/sync_duration_p50_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    echo "Querying network programming duration..."
    query_prometheus_range \
        'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))' \
        "${RESULT_DIR}/network_programming_p99_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    echo "Querying CPU usage..."
    query_prometheus_range \
        'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))' \
        "${RESULT_DIR}/cpu_usage_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    echo "Querying memory usage..."
    query_prometheus_range \
        'sum(process_resident_memory_bytes{job="kube-proxy"})' \
        "${RESULT_DIR}/memory_usage_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    echo "Querying API server latency..."
    query_prometheus_range \
        'histogram_quantile(0.99, sum by (le) (rate(apiserver_request_duration_seconds_bucket{verb="PATCH",resource=~"endpoints|endpointslices"}[1m])))' \
        "${RESULT_DIR}/apiserver_latency_p99_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    echo "Querying sync rule counts..."
    query_prometheus_range \
        'sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count[1m]))' \
        "${RESULT_DIR}/sync_count_timeseries.json" \
        "$start_time" "$end_time" "5"
    
    # Final state metrics
    query_prometheus 'count(kube_pod_info{pod=~"massive-scale-deployment.*"})' \
        "${RESULT_DIR}/final_pod_count.json"
    
    echo "[$(date +%T)] Deployment metrics collected"
}

# Function to collect cluster state
collect_cluster_state() {
    echo ""
    echo "[$(date +%T)] Collecting cluster state..."
    
    kubectl get nodes -o wide > "${RESULT_DIR}/nodes.txt"
    # Capture pods dynamically assigned to ANY KWOK fake node (kwok-node-*)
    kubectl get nodes -l type=kwok -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | while read node; do
        kubectl get pods --all-namespaces --field-selector spec.nodeName=$node --no-headers 2>/dev/null | wc -l
    done | awk '{sum+=$1} END {print sum}' > "${RESULT_DIR}/kwok_pods_count.txt"
    kubectl get deployment massive-scale-deployment -o yaml > "${RESULT_DIR}/deployment_state.yaml" 2>/dev/null || true
    kubectl get service massive-scale-service -o yaml > "${RESULT_DIR}/service_state.yaml" 2>/dev/null || true
    kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o yaml > "${RESULT_DIR}/endpointslices_state.yaml" 2>/dev/null || true
    
    echo "[$(date +%T)] Cluster state collected"
}

# Function to generate summary report
generate_summary() {
    echo ""
    echo "[$(date +%T)] Generating summary report..."
    
    cat > "${RESULT_DIR}/SUMMARY.md" <<EOF
# Kube-Proxy Mode Comparison Experiment Results

## Experiment Configuration
- **Mode**: $MODE
- **Replicas**: $REPLICAS
- **Duration**: ${DURATION}s
- **Timestamp**: $TIMESTAMP
- **Start Time**: $(date -d @$(cat ${RESULT_DIR}/deploy_start_timestamp.txt) 2>/dev/null || echo "N/A")
- **End Time**: $(date -d @$(cat ${RESULT_DIR}/deploy_end_timestamp.txt) 2>/dev/null || echo "N/A")

## Key Metrics Collected

### Kube-Proxy Performance
- \`sync_duration_p99_timeseries.json\` - 99th percentile sync duration
- \`network_programming_p99_timeseries.json\` - Network programming latency

### Resource Usage
- \`cpu_usage_timeseries.json\` - CPU consumption

## PromQL Queries Used (Corrected)

\`\`\`promql
# 99th percentile sync duration
histogram_quantile(0.99, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))

# Network programming duration
histogram_quantile(0.99, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))

# CPU usage
sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))

# API server latency
histogram_quantile(0.99, sum by (le) (rate(apiserver_request_duration_seconds_bucket{verb="PATCH",resource=~"endpoints|endpointslices"}[1m])))
\`\`\`
EOF

    echo "[$(date +%T)] Summary report generated: ${RESULT_DIR}/SUMMARY.md"
}

# Function to cleanup deployment
cleanup_deployment() {
    if [[ "$CLEANUP" == true ]]; then
        echo ""
        echo "[$(date +%T)] Cleaning up deployment..."
        kubectl delete -f "${RESULT_DIR}/deployment.yaml" --ignore-not-found=true
        echo "[$(date +%T)] Cleanup complete"
    else
        echo ""
        echo "Deployment left running. To clean up manually, run:"
        echo "  kubectl delete -f ${RESULT_DIR}/deployment.yaml"
    fi
}

# Main execution flow
main() {
    verify_proxy_mode
    collect_baseline_metrics
    prepare_deployment
    apply_deployment
    monitor_deployment
    collect_deployment_metrics
    collect_cluster_state
    generate_summary
    cleanup_deployment
    
    echo ""
    echo "========================================"
    echo "Experiment Complete!"
    echo "========================================"
    echo "Results saved to: $RESULT_DIR"
    echo ""
}

# Run the experiment
main