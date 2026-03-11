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
DEPLOYMENT_YAML="${SCRIPT_DIR}/deployment-only.yaml"
SERVICE_YAML="${SCRIPT_DIR}/service-only.yaml"

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

if [[ ! -f "$DEPLOYMENT_YAML" ]] || [[ ! -f "$SERVICE_YAML" ]]; then
    echo "Error: Two-Step Tsunami architecture requires TWO separate files."
    echo "Missing: $DEPLOYMENT_YAML or $SERVICE_YAML"
    echo "Please split your old massive-scale-deployment.yaml into these two files."
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
        kubectl delete service massive-scale-service --ignore-not-found=true --wait=false 2>/dev/null || true
        
        # 2. Polling loop instead of kubectl wait
        echo "[$(date +%T)] Polling for pod termination status..."
        local max_checks=30 # 150 seconds max wait (30 * 5s)
        local i=0
        while [ $i -lt $max_checks ]; do
            local remaining_pods=$(kubectl get pods -l app=fake-workload -o name 2>/dev/null | wc -l || echo "0")
            if [ "$remaining_pods" -eq 0 ]; then
                break
            fi
            echo "[$(date +%T)] $remaining_pods pods still terminating..."
            sleep 5
            i=$((i + 1))
        done
        
        # 3. Non-blocking force delete prevents script lockup
        local final_pods=$(kubectl get pods -l app=fake-workload -o name 2>/dev/null | wc -l || echo "0")
        if [ "$final_pods" -gt 0 ]; then
            echo "WARNING: $final_pods pods stuck. Force-deleting in the background..."
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

prewarm_pods() {
    local replicas="$1"
    local result_dir="$2"
    local is_first_iteration="$3"
    
    if [[ "$is_first_iteration" == "true" ]]; then
        echo -e "\n[$(date +%T)] PHASE 1: Pre-warming infrastructure (0 replicas initially)..."
        # First iteration: Create deployment with 0 replicas
        sed "s/replicas: .*/replicas: 0/" "${DEPLOYMENT_YAML}" > "${result_dir}/deployment.yaml"
        echo "[$(date +%T)] Creating initial deployment with 0 replicas (NO service yet)..."
        kubectl apply -f "${result_dir}/deployment.yaml"
        echo "[$(date +%T)] Deployment ready at 0 replicas. Service will be created next."
        return 0
    fi
    
    echo -e "\n[$(date +%T)] PHASE 1: Pre-warming deployment for $replicas replicas..."
    # Subsequent iterations: Scale existing deployment
    echo "[$(date +%T)] Scaling existing deployment from $(kubectl get deployment massive-scale-deployment -o jsonpath='{.spec.replicas}' 2>/dev/null || echo '?') to ${replicas} replicas..."
    kubectl scale deployment massive-scale-deployment --replicas=${replicas}
    
    local timeout=3600 # 60 mins hard safety for pod creation
    echo "[$(date +%T)] Monitoring until ${replicas} pods are scheduled and Running (Timeout: ${timeout}s)..."
    
    local end_time=$(($(date +%s) + timeout))
    local interval=10
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local pods_ready=$(kubectl get deployment massive-scale-deployment -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo "0")
        pods_ready=${pods_ready:-0}
        
        if [[ "$pods_ready" -ge "$replicas" ]]; then
            echo "[$(date +%T)] All ${replicas} pods are Running and Ready. Control Plane pre-warming complete."
            return 0
        fi
        
        echo "[$(date +%T)] Pods Ready: ${pods_ready}/${replicas} (Waiting for K8s API & Scheduler...)"
        sleep $interval
    done
    
    echo "Error: Pod pre-warming timed out!"
    exit 1
}

trigger_tsunami_and_monitor() {
    local result_dir="$1"
    local target_replicas="$2"
    local is_first_iteration="$3"
    
    if [[ "$is_first_iteration" == "true" ]]; then
        echo -e "\n[$(date +%T)] PHASE 2: SERVICE SETUP + SCALING (0 → ${target_replicas})"
        echo "[$(date +%T)] Creating Service with 0 endpoints..."
        kubectl apply -f "${SERVICE_YAML}"
        sleep 5  # Brief wait for service to stabilize
        
        echo "[$(date +%T)] Scaling deployment from 0 to ${target_replicas} replicas..."
        echo "[$(date +%T)] This is the measured tsunami event!"
        
        # Start the clock EXACTLY before scaling
        local deploy_start=$(date +%s)
        echo "$deploy_start" > "${result_dir}/deploy_start_timestamp.txt"
        
        kubectl scale deployment massive-scale-deployment --replicas=${target_replicas}
    else
        echo -e "\n[$(date +%T)] PHASE 2: INCREMENTAL SCALE EVENT (Service already exists)"
        echo "[$(date +%T)] Service is already running. Monitoring kube-proxy's incremental update..."
        
        # Start the clock EXACTLY when pods became ready (just before this function)
        local deploy_start=$(date +%s)
        echo "$deploy_start" > "${result_dir}/deploy_start_timestamp.txt"
    fi
    
    local timeout=1200 # 20 mins max for proxy catch-up
    local end_time=$(($(date +%s) + timeout))
    local interval=5
    
    echo -e "\n[$(date +%T)] Monitoring kube-proxy data-plane sync..."
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local endpoints=$(kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o jsonpath='{.items[*].endpoints[*].addresses[*]}' 2>/dev/null | wc -w || echo "0")
        
        echo "[$(date +%T)] IP Endpoints Grouped: ${endpoints}/${target_replicas}"
        
        if [[ "$endpoints" -ge "$target_replicas" ]]; then
            echo "[$(date +%T)] EndpointSlices fully updated to ${target_replicas} endpoints."
            
            echo "[$(date +%T)] Waiting for kube-proxy to finish writing rules to the Data Plane..."
            local max_proxy_wait=120 # 10 minutes max just for proxy catch-up
            local proxy_wait=0
            
            while [ $proxy_wait -lt $max_proxy_wait ]; do
                local sync_rate=$(curl -s -G "${PROMETHEUS_URL}/api/v1/query" \
                    --data-urlencode 'query=sum(increase(kubeproxy_sync_proxy_rules_duration_seconds_count[15s]))' | \
                    grep -oP '"value":\[[^,]+,"([^"]+)"\]' | grep -oP ',"([^"]+)"' | tr -d ',"' || echo "1.0")
                
                # Check for rate flattening
                if [[ "$sync_rate" == 0.0* ]] || [[ "$sync_rate" == "0" ]]; then
                    echo "[$(date +%T)] Data Plane Sync complete! (kube-proxy sync rate normalized: $sync_rate)"
                    break 2 # Break out of both the proxy loop and the endpoint loop
                fi
                
                echo "[$(date +%T)] kube-proxy is continuously syncing rules (Current rate: $sync_rate syncs/sec)..."
                sleep 5
                proxy_wait=$((proxy_wait + 1))
            done
            break
        fi
        
        sleep $interval
    done
    
    echo "[$(date +%T)] Waiting an additional 30 seconds for trailing metric collection..."
    sleep 30
    
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
    local iter_num="$2"
    local total_iters="$3"
    
    echo -e "\n========================================"
    echo "Iteration ${iter_num}/${total_iters}: ${replicas} replicas (Staircase Scale-Up)"
    echo "========================================"
    
    local iter_dir="${BASE_RESULT_DIR}/replicas_${replicas}"
    mkdir -p "$iter_dir"
    
    # Determine if this is the first iteration
    local is_first_iteration="false"
    if [[ "$iter_num" -eq 1 ]]; then
        is_first_iteration="true"
    fi
    
    # NO cleanup between iterations - this is critical for measuring incremental updates!
    
    prewarm_pods "$replicas" "$iter_dir" "$is_first_iteration"
    collect_baseline_metrics "$iter_dir"
    trigger_tsunami_and_monitor "$iter_dir" "$replicas" "$is_first_iteration"
    
    collect_deployment_metrics "$iter_dir" "$replicas"
    collect_cluster_state "$iter_dir"
    generate_iteration_summary "$iter_dir" "$replicas"
    
    echo "[$(date +%T)] Iteration ${iter_num}/${total_iters} complete"
}

# --- Main Execution Flow ---

main() {
    verify_proxy_mode
    
    local total_iters=${#REPLICA_ITERATIONS[@]}
    local iter_num=1
    
    for replicas in "${REPLICA_ITERATIONS[@]}"; do
        run_iteration "$replicas" "$iter_num" "$total_iters"
        iter_num=$((iter_num + 1))
    done
    
    echo -e "\n[$(date +%T)] Performing final cleanup (tearing down all resources)..."
    cleanup_deployment 10
    
    generate_overall_summary
    
    echo -e "\n========================================"
    echo "All Iterations Complete!"
    echo "Results saved to: $BASE_RESULT_DIR"
    echo "========================================"
    echo "Continuous Staircase Methodology:"
    echo "  - Service remained active across all iterations"
    echo "  - Each scale event measured pure incremental update performance"
    echo "  - nftables: Should show flat latency (O(1) incremental)"
    echo "  - iptables: Should show exponential curve (O(N) full rewrite)"
    echo "========================================"
}

main