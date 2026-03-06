#!/bin/bash
################################################################################
# Single Kube-Proxy Mode Comparison Experiment
# 
# This script automates a single control plane latency experiment comparing
# iptables vs nftables modes for kube-proxy, with a user-defined replica count.
################################################################################

set -euo pipefail

# Default values
MODE=""
REPLICAS=""
DURATION=60
PROMETHEUS_URL="http://localhost:9090"
OUTPUT_DIR="./results"
CLEANUP_WAIT=60
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOYMENT_YAML="${SCRIPT_DIR}/massive-scale-deployment.yaml"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode) MODE="$2"; shift 2 ;;
        --replicas) REPLICAS="$2"; shift 2 ;;
        --duration) DURATION="$2"; shift 2 ;;
        --prometheus-url) PROMETHEUS_URL="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        --cleanup-wait) CLEANUP_WAIT="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Validate required arguments
if [[ -z "$MODE" ]] || [[ "$MODE" != "iptables" && "$MODE" != "nftables" ]]; then
    echo "Error: --mode is required and must be either 'iptables' or 'nftables'"
    echo "Usage: $0 --mode [iptables|nftables] --replicas [NUMBER] [OPTIONS]"
    exit 1
fi

if [[ -z "$REPLICAS" ]] || ! [[ "$REPLICAS" =~ ^[0-9]+$ ]]; then
    echo "Error: --replicas is required and must be a number"
    echo "Usage: $0 --mode [iptables|nftables] --replicas [NUMBER] [OPTIONS]"
    exit 1
fi

if [[ ! -f "$DEPLOYMENT_YAML" ]]; then
    echo "Error: Base deployment file not found at $DEPLOYMENT_YAML"
    exit 1
fi

# Create base output directory
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULT_DIR="${OUTPUT_DIR}/${MODE}_${REPLICAS}pods_${TIMESTAMP}"
mkdir -p "$RESULT_DIR"

echo "========================================"
echo "Single Kube-Proxy Mode Experiment"
echo "========================================"
echo "Mode:            $MODE"
echo "Replicas:        $REPLICAS"
echo "Duration:        ${DURATION}s"
echo "Cleanup wait:    ${CLEANUP_WAIT}s"
echo "Prometheus URL:  $PROMETHEUS_URL"
echo "Output Dir:      $RESULT_DIR"
echo "========================================"

# --- Helper Functions ---

query_prometheus() {
    local query="$1"
    local output_file="$2"
    local timestamp=$(date +%s)
    curl -s -G "${PROMETHEUS_URL}/api/v1/query" \
        --data-urlencode "query=${query}" \
        --data-urlencode "time=${timestamp}" \
        -o "${output_file}" || echo "Warning: Failed to query Prometheus"
}

query_prometheus_range() {
    local query="$1"
    local output_file="$2"
    local start="$3"
    local end="$4"
    local step="${5:-5}"
    curl -s -G "${PROMETHEUS_URL}/api/v1/query_range" \
        --data-urlencode "query=${query}" \
        --data-urlencode "start=${start}" \
        --data-urlencode "end=${end}" \
        --data-urlencode "step=${step}" \
        -o "${output_file}" || echo "Warning: Failed to query Prometheus range"
}

verify_proxy_mode() {
    echo -e "\n[$(date +%T)] Verifying kube-proxy mode..."
    set +e
    local actual_mode=$(kubectl -n kube-system get cm kube-proxy -o jsonpath='{.data.config\.conf}' 2>/dev/null | grep -E '^\s*mode:' | awk '{print $2}' | tr -d '"' || echo "")
    set -e
    
    if [[ -z "$actual_mode" ]]; then
        actual_mode="iptables"
    fi

    echo "Expected mode: $MODE"
    echo "Actual mode:   $actual_mode"

    if [[ "$actual_mode" != "$MODE" ]]; then
        echo "WARNING: kube-proxy mode mismatch! Expected ${MODE} but detected ${actual_mode}."
        read -p "Continue anyway? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
    echo "$actual_mode" > "${RESULT_DIR}/proxy_mode.txt"
}

cleanup_deployment() {
    local wait_time="$1"
    local cleanup_script="${SCRIPT_DIR}/cleanup.sh"
    
    if [[ -f "$cleanup_script" ]]; then
        bash "$cleanup_script" "$wait_time"
    else
        echo -e "\n[$(date +%T)] Cleaning up deployment..."
        kubectl delete deployment massive-scale-deployment --ignore-not-found=true
        kubectl delete service massive-scale-service --ignore-not-found=true
        
        echo "[$(date +%T)] Waiting for pods to terminate..."
        kubectl wait --for=delete pod -l app=fake-workload --timeout=120s 2>/dev/null || true
        
        local remaining_pods=$(kubectl get pods -l app=fake-workload --no-headers 2>/dev/null | wc -l || echo "0")
        if [[ "$remaining_pods" -gt 0 ]]; then
            kubectl delete pods -l app=fake-workload --force --grace-period=0 2>/dev/null || true
        fi

        echo "[$(date +%T)] Waiting ${wait_time}s for network rules to settle..."
        sleep "$wait_time"
    fi
}

collect_baseline_metrics() {
    local result_dir="$1"
    echo -e "\n[$(date +%T)] Collecting baseline metrics..."
    query_prometheus 'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))' "${result_dir}/baseline_cpu.json"
    query_prometheus 'sum(process_resident_memory_bytes{job="kube-proxy"})' "${result_dir}/baseline_memory.json"
    query_prometheus 'count(kube_pod_info)' "${result_dir}/baseline_pod_count.json"
    query_prometheus 'count(kube_service_info)' "${result_dir}/baseline_service_count.json"
}

prepare_and_apply_deployment() {
    local replicas="$1"
    local result_dir="$2"
    
    echo -e "\n[$(date +%T)] Preparing deployment for $replicas replicas..."
    sed "s/replicas: .*/replicas: ${replicas}/" "${DEPLOYMENT_YAML}" > "${result_dir}/deployment.yaml"
    
    echo "[$(date +%T)] Applying massive-scale deployment..."
    local deploy_start=$(date +%s)
    echo "$deploy_start" > "${result_dir}/deploy_start_timestamp.txt"
    kubectl apply -f "${result_dir}/deployment.yaml"
}

monitor_deployment() {
    local result_dir="$1"
    local duration="$2"
    
    echo -e "\n[$(date +%T)] Monitoring deployment for ${duration} seconds..."
    
    local end_time=$(($(date +%s) + duration))
    local interval=10
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local pods_ready=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.readyReplicas}' 2>/dev/null)
        local pods_total=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.replicas}' 2>/dev/null)
        
        pods_ready=${pods_ready:-0}
        pods_total=${pods_total:-0}
        local endpoints=$(kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o jsonpath='{.items[*].endpoints[*].addresses[*]}' 2>/dev/null | wc -w || echo "0")
        
        echo "[$(date +%T)] Pods: ${pods_ready}/${pods_total} | Endpoints: ${endpoints}"
        sleep $interval
    done
    
    date +%s > "${result_dir}/deploy_end_timestamp.txt"
}

collect_deployment_metrics() {
    local result_dir="$1"
    echo -e "\n[$(date +%T)] Collecting deployment metrics..."
    
    local start_time=$(cat "${result_dir}/deploy_start_timestamp.txt")
    local end_time=$(cat "${result_dir}/deploy_end_timestamp.txt")
    
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' "${result_dir}/sync_duration_p99_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' "${result_dir}/sync_duration_p50_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))' "${result_dir}/network_programming_p99_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))' "${result_dir}/cpu_usage_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'sum(process_resident_memory_bytes{job="kube-proxy"})' "${result_dir}/memory_usage_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(apiserver_request_duration_seconds_bucket{verb="PATCH",resource=~"endpoints|endpointslices"}[1m])))' "${result_dir}/apiserver_latency_p99_timeseries.json" "$start_time" "$end_time" "5"
    query_prometheus_range 'sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count[1m]))' "${result_dir}/sync_count_timeseries.json" "$start_time" "$end_time" "5"
    
    query_prometheus 'count(kube_pod_info{pod=~"massive-scale-deployment.*"})' "${result_dir}/final_pod_count.json"
}

collect_cluster_state() {
    local result_dir="$1"
    echo -e "\n[$(date +%T)] Collecting cluster state..."
    
    kubectl get nodes -o wide > "${result_dir}/nodes.txt" 2>/dev/null || true
    
    kubectl get nodes -l type=kwok -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | while read node; do
        [[ -n "$node" ]] && kubectl get pods --all-namespaces --field-selector spec.nodeName=$node --no-headers 2>/dev/null | wc -l || echo "0"
    done | awk '{sum+=$1} END {print sum}' > "${result_dir}/kwok_pods_count.txt"
    
    kubectl get deployment massive-scale-deployment -o yaml > "${result_dir}/deployment_state.yaml" 2>/dev/null || true
    kubectl get service massive-scale-service -o yaml > "${result_dir}/service_state.yaml" 2>/dev/null || true
    kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o yaml > "${result_dir}/endpointslices_state.yaml" 2>/dev/null || true
}

generate_summary() {
    local result_dir="$1"
    local replicas="$2"
    local duration="$3"
    echo "[$(date +%T)] Generating summary..."
    
    cat > "${result_dir}/SUMMARY.md" <<EOF
# Single Kube-Proxy Mode Experiment - ${replicas} Replicas

## Experiment Configuration
- **Mode**: $MODE
- **Replicas**: $replicas
- **Duration**: ${duration}s
- **Start Time**: $(date -d @$(cat ${result_dir}/deploy_start_timestamp.txt 2>/dev/null || echo 0) 2>/dev/null || echo "N/A")
- **End Time**: $(date -d @$(cat ${result_dir}/deploy_end_timestamp.txt 2>/dev/null || echo 0) 2>/dev/null || echo "N/A")

## Collected Metrics
Check the JSON files in this directory for timeseries data regarding:
- Sync Duration (p99, p50)
- Network Programming Latency (p99)
- CPU and Memory Consumption
- API Server Latency
EOF
}

# --- Main Execution Flow ---

main() {
    verify_proxy_mode
    
    # Optional initial cleanup
    cleanup_deployment "$CLEANUP_WAIT"
    
    collect_baseline_metrics "$RESULT_DIR"
    prepare_and_apply_deployment "$REPLICAS" "$RESULT_DIR"
    monitor_deployment "$RESULT_DIR" "$DURATION"
    collect_deployment_metrics "$RESULT_DIR"
    collect_cluster_state "$RESULT_DIR"
    generate_summary "$RESULT_DIR" "$REPLICAS" "$DURATION"
    
    echo -e "\n[$(date +%T)] Performing final cleanup..."
    cleanup_deployment 10
    
    echo -e "\n========================================"
    echo "Experiment Complete!"
    echo "Results saved to: $RESULT_DIR"
    echo "========================================"
}

main
