#!/bin/bash
################################################################################
# Kube-Proxy Mode Comparison Experiment
# 
# This script automates the control plane latency experiment comparing
# iptables vs nftables modes for kube-proxy.
################################################################################

set -euo pipefail

# Default values
MODE=""
PROMETHEUS_URL="http://localhost:9090"
OUTPUT_DIR="./results"
REPLICA_ITERATIONS=(100 300 1000 5000 10000 20000 30000)
CLEANUP_WAIT=60
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMBINED_YAML="${SCRIPT_DIR}/massive-scale.yaml"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --mode) MODE="$2"; shift 2 ;;
        --prometheus-url) PROMETHEUS_URL="$2"; shift 2 ;;
        --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
        --cleanup-wait) CLEANUP_WAIT="$2"; shift 2 ;;
        *) echo "Unknown option: $1"; exit 1 ;;
    esac
done

# Validate required arguments
if [[ -z "$MODE" ]] || [[ "$MODE" != "iptables" && "$MODE" != "nftables" ]]; then
    echo "Error: --mode is required and must be either 'iptables' or 'nftables'"
    echo "Usage: $0 --mode [iptables|nftables] [OPTIONS]"
    exit 1
fi

if [[ ! -f "$COMBINED_YAML" ]]; then
    echo "Error: Missing combined YAML file."
    echo "Missing: $COMBINED_YAML"
    exit 1
fi

# Create base output directory
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BASE_RESULT_DIR="${OUTPUT_DIR}/${MODE}_${TIMESTAMP}"
mkdir -p "$BASE_RESULT_DIR"

echo "========================================"
echo "Kube-Proxy Mode Comparison Experiment"
echo "========================================"
echo "Mode:            $MODE"
echo "Iterations:      ${REPLICA_ITERATIONS[*]}"
echo "Architecture:    Continuous Staircase (Service persists across scales)"
echo "Methodology:     Measures incremental kube-proxy updates (constant delta)"
echo "Prometheus URL:  $PROMETHEUS_URL"
echo "Output Dir:      $BASE_RESULT_DIR"
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
    # Disable exit-on-error temporarily for this check
    set +e
    local actual_mode=$(kubectl -n kube-system get cm kube-proxy -o jsonpath='{.data.config\.conf}' 2>/dev/null | grep -E '^\s*mode:' | awk '{print $2}' | tr -d '"' || echo "")
    set -e
    
    # If mode is empty or not set, kube-proxy defaults to iptables
    if [[ -z "$actual_mode" ]]; then
        actual_mode="iptables"
    fi

    echo "Expected mode: $MODE"
    echo "Actual mode:   $actual_mode"

    if [[ "$actual_mode" != "$MODE" ]]; then
        echo "WARNING: kube-proxy mode mismatch! You specified --mode=$MODE but it appears to be $actual_mode."
        read -p "Continue anyway? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi
    echo "$actual_mode" > "${BASE_RESULT_DIR}/proxy_mode.txt"
}

cleanup_deployment() {
    local wait_time="$1"
    
    # Call the external cleanup script to keep logic DRY and modular
    local cleanup_script="${SCRIPT_DIR}/cleanup.sh"
    
    if [[ -f "$cleanup_script" ]]; then
        bash "$cleanup_script" "$wait_time"
    else
        echo "WARNING: cleanup.sh not found at $cleanup_script, performing inline cleanup..."
        echo -e "\n[$(date +%T)] Cleaning up deployment (Async Mode)..."
        
        # 1. Use --wait=false to avoid the API server hanging your terminal
        kubectl delete deployment massive-scale-deployment --ignore-not-found=true --wait=false 2>/dev/null || true
        kubectl get deployments -o name | grep massive-delta | xargs -r kubectl delete --ignore-not-found=true --wait=false 2>/dev/null || true
        kubectl delete service massive-scale-service --ignore-not-found=true --wait=false 2>/dev/null || true
        
        # 2. Polling loop instead of kubectl wait
        echo "[$(date +%T)] Polling for pod termination status..."
        local max_checks=30 # 150 seconds max wait (30 * 5s)
        local i=0
        while [ $i -lt $max_checks ]; do
            local remaining_pods=$(kubectl get pods -l 'delta-id' -o name 2>/dev/null | wc -l || echo "0")
            local remaining_pods_old=$(kubectl get pods -l app=fake-workload -o name 2>/dev/null | wc -l || echo "0")
            local total_rem=$((remaining_pods + remaining_pods_old))
            if [ "$total_rem" -eq 0 ]; then
                break
            fi
            echo "[$(date +%T)] $total_rem pods still terminating..."
            sleep 5
            i=$((i + 1))
        done
        
        # 3. Non-blocking force delete prevents script lockup
        local final_pods=$(kubectl get pods -l 'delta-id' -o name 2>/dev/null | wc -l || echo "0")
        if [ "$final_pods" -gt 0 ]; then
            echo "WARNING: $final_pods pods stuck. Force-deleting in the background..."
            nohup kubectl delete pods -l 'delta-id' --force --grace-period=0 >/dev/null 2>&1 &
            nohup kubectl delete pods -l app=fake-workload --force --grace-period=0 >/dev/null 2>&1 &
            sleep 10
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

scale_and_monitor() {
    local target_replicas="$1"
    local result_dir="$2"
    
    echo -e "\n[$(date +%T)] PHASE: DATA-PLANE SCALING (RELATIVE BASELINE)"
    
    # Start the clock EXACTLY before applying the components
    local deploy_start=$(date +%s)
    echo "$deploy_start" > "${result_dir}/deploy_start_timestamp.txt"
    
    echo "[$(date +%T)] Applying Combined YAML to update replicas: ${target_replicas}..."
    sed "s/replicas: .*/replicas: ${target_replicas}/" "${COMBINED_YAML}" > "${result_dir}/massive-scale.yaml"
    kubectl apply -f "${result_dir}/massive-scale.yaml"
    
    local timeout=3600 # 60 mins hard safety
    local end_time=$(($(date +%s) + timeout))
    local interval=5
    local settle_count=0
    local settling_phase="false"
    
    echo -e "\n[$(date +%T)] Monitoring pod creation and kube-proxy data-plane sync simultaneously..."
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local pods_ready=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        pods_ready=${pods_ready:-0}
        
        local endpoints=$(kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o jsonpath='{.items[*].endpoints[*].addresses[*]}' 2>/dev/null | wc -w || echo "0")
        
        if [[ "$settling_phase" == "false" ]]; then
            echo "[$(date +%T)] Pods: ${pods_ready}/${target_replicas} | Endpoints: ${endpoints}/${target_replicas}"
        fi
        
        if [[ "$pods_ready" -ge "$target_replicas" ]] && [[ "$endpoints" -ge "$target_replicas" ]]; then
            if [[ "$settling_phase" == "false" ]]; then
                echo "[$(date +%T)] Target rules submitted to API! Dynamically monitoring kube-proxy CPU to detect sync completion..."
                settling_phase="true"
            fi
            
            # Query Prometheus for proxy sync count using a 5s rate window
            # If the rate is > 0, it means kube-proxy is actively flushing sync cycles
            local sync_query='sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count[5s]))'
            local response=$(curl -s -G "${PROMETHEUS_URL}/api/v1/query" --data-urlencode "query=${sync_query}" 2>/dev/null || echo "")
            
            # Parse the float value safely using inline python
            local current_sync_rate="0.0"
            if [[ -n "$response" ]]; then
                current_sync_rate=$(echo "$response" | python -c "import sys, json; res=json.load(sys.stdin).get('data',{}).get('result',[]); print(float(res[0]['value'][1]) if res else 0.0)" 2>/dev/null || echo "0.0")
            fi
            
            # Float comparison via awk (check if sync rate > 0.05 syncs per second)
            local is_syncing=$(awk -v rate="$current_sync_rate" 'BEGIN { if (rate > 0.05) print "1"; else print "0" }')
            
            if [[ "$is_syncing" == "1" ]]; then
                echo "[$(date +%T)] kube-proxy still syncing rules (Sync Rate: ${current_sync_rate} syncs/sec). Waiting..."
                settle_count=0
            else
                settle_count=$((settle_count + 1))
                echo "[$(date +%T)] kube-proxy sync rate dropping (Sync Rate: ${current_sync_rate} syncs/sec). Settling... (${settle_count}/2)"
                if [[ "$settle_count" -ge 2 ]]; then
                    echo "[$(date +%T)] kube-proxy has fully settled! Data plane sync complete."
                    sleep 5 # Brief buffer to ensure Prometheus scraped the final drop
                    break
                fi
            fi
        fi
        
        sleep $interval
    done
    
    date +%s > "${result_dir}/deploy_end_timestamp.txt"
}

collect_deployment_metrics() {
    local result_dir="$1"
    local replicas_count="$2"
    echo -e "\n[$(date +%T)] Collecting deployment metrics..."
    
    local start_time=$(cat "${result_dir}/deploy_start_timestamp.txt")
    local end_time=$(cat "${result_dir}/deploy_end_timestamp.txt")
    
    # Use a stable 10s query resolution for all iterations
    local step="10"
    
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' "${result_dir}/sync_duration_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.95, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' "${result_dir}/sync_duration_p95_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m])))' "${result_dir}/sync_duration_p50_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))' "${result_dir}/network_programming_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.95, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))' "${result_dir}/network_programming_p95_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[1m])))' "${result_dir}/network_programming_p50_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))' "${result_dir}/cpu_usage_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(process_resident_memory_bytes{job="kube-proxy"})' "${result_dir}/memory_usage_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(apiserver_request_duration_seconds_bucket{verb=~"POST|PUT|PATCH",resource=~"endpoints.*"}[1m])))' "${result_dir}/apiserver_latency_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count[1m]))' "${result_dir}/sync_count_timeseries.json" "$start_time" "$end_time" "$step"
    
    # Final state metrics
    query_prometheus 'count(kube_pod_info{pod=~"massive-scale-deployment.*"})' "${result_dir}/final_pod_count.json"
}

collect_cluster_state() {
    local result_dir="$1"
    echo -e "\n[$(date +%T)] Collecting cluster state..."
    
    kubectl get nodes -o wide > "${result_dir}/nodes.txt" 2>/dev/null || true
    
    # Count pods on KWOK fake nodes
    kubectl get nodes -l type=kwok -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | while read node; do
        [[ -n "$node" ]] && kubectl get pods --all-namespaces --field-selector spec.nodeName=$node --no-headers 2>/dev/null | wc -l || echo "0"
    done | awk '{sum+=$1} END {print sum}' > "${result_dir}/kwok_pods_count.txt"
    
    kubectl get deployment massive-scale-deployment -o yaml > "${result_dir}/deployment_state.yaml" 2>/dev/null || true
    kubectl get service massive-scale-service -o yaml > "${result_dir}/service_state.yaml" 2>/dev/null || true
    kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o yaml > "${result_dir}/endpointslices_state.yaml" 2>/dev/null || true
}

generate_iteration_summary() {
    local result_dir="$1"
    local replicas="$2"
    echo "[$(date +%T)] Generating iteration summary..."
    
    cat > "${result_dir}/SUMMARY.md" <<EOF
# Kube-Proxy Mode Comparison - ${replicas} Replicas

## Experiment Configuration
- **Mode**: $MODE
- **Replicas**: $replicas
- **Duration**: Dynamic (until target pods are ready + 30s buffer)
- **Start Time**: $(date -d @$(cat ${result_dir}/deploy_start_timestamp.txt 2>/dev/null || echo 0) 2>/dev/null || echo "N/A")
- **End Time**: $(date -d @$(cat ${result_dir}/deploy_end_timestamp.txt 2>/dev/null || echo 0) 2>/dev/null || echo "N/A")

## Collected Metrics
Check the JSON files in this directory for timeseries data regarding:
- Sync Duration (p99, p95, p50)
- Network Programming Latency (p99, p95, p50)
- CPU and Memory Consumption
- API Server Latency
EOF
}

generate_overall_summary() {
    echo -e "\n[$(date +%T)] Generating overall summary..."
    local summary_file="${BASE_RESULT_DIR}/OVERALL_SUMMARY.md"
    
    cat > "$summary_file" <<EOF
# Overall Experiment Summary: $MODE Mode

## Iterations Run
EOF

    for replicas in "${REPLICA_ITERATIONS[@]}"; do
        if [[ -d "${BASE_RESULT_DIR}/replicas_${replicas}" ]]; then
            echo "- [${replicas} Replicas](replicas_${replicas}/SUMMARY.md)" >> "$summary_file"
        fi
    done

    cat >> "$summary_file" <<EOF

## Analysis Instructions
Compare results across iterations to observe how kube-proxy performance scales with increasing pod counts:
1. **Sync Duration Trends**: Check if sync latency increases linearly or exponentially.
2. **Resource Usage**: Monitor CPU and memory growth patterns.
3. **Network Programming**: Observe end-to-end latency scaling.
4. **API Server Load**: Identify if API becomes a bottleneck.
EOF
}

run_iteration() {
    local replicas="$1"
    local prev_replicas="$2"
    local iter_num="$3"
    local total_iters="$4"
    
    echo -e "\n========================================"
    echo "Iteration ${iter_num}/${total_iters}: Scaling from ${prev_replicas} to ${replicas} (Relative Baseline)"
    echo "========================================"
    
    local iter_dir="${BASE_RESULT_DIR}/replicas_${replicas}"
    mkdir -p "$iter_dir"
    
    collect_baseline_metrics "$iter_dir"
    scale_and_monitor "$replicas" "$iter_dir"
    
    collect_deployment_metrics "$iter_dir" "$replicas"
    collect_cluster_state "$iter_dir"
    generate_iteration_summary "$iter_dir" "$replicas"
    
    echo "[$(date +%T)] Iteration ${iter_num}/${total_iters} complete. Keeping resources active for next phase."
}

# --- Main Execution Flow ---

main() {
    verify_proxy_mode
    
    local total_iters=${#REPLICA_ITERATIONS[@]}
    local iter_num=1
    local prev_replicas=0
    
    for replicas in "${REPLICA_ITERATIONS[@]}"; do
        run_iteration "$replicas" "$prev_replicas" "$iter_num" "$total_iters"
        prev_replicas=$replicas
        iter_num=$((iter_num + 1))
    done
    
    echo -e "\n[$(date +%T)] Performing final cleanup (tearing down all resources)..."
    cleanup_deployment 10
    
    generate_overall_summary
    
    echo -e "\n========================================"
    echo "All Iterations Complete!"
    echo "Results saved to: $BASE_RESULT_DIR"
    echo "========================================"
    echo "Organic Bundled Methodology:"
    echo "  - Service and Deployment are actively paired."
    echo "  - Replicas are scaled up normally using kubectl apply."
    echo "  - Endpoints trickle in organically, simulating real autoscaling."
    echo "  - Accurately captures iptables CPU thrashing and rule-cycle duration."
    echo "========================================"
}

main