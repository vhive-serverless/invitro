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

delta_tsunami_and_monitor() {
    local target_replicas="$1"
    local prev_replicas="$2"
    local iter_num="$3"
    local total_iters="$4"
    local result_dir="$5"
    
    local delta=$(( target_replicas - prev_replicas ))
    
    echo -e "\n[$(date +%T)] PHASE 1: PRE-WARMING DELTA PODS ($delta pods)"
    
    if [[ "$delta" -gt 0 ]]; then
        echo "[$(date +%T)] Attempting to pre-warm $delta pods invisibly using boolean iteration matching..."
        
        # Dynamically generate labels so this delta pod matches this iteration and ALL future iterations
        local dynamic_labels=""
        for (( i=$iter_num; i<=$total_iters; i++ )); do
            dynamic_labels+="        match-${i}: \"yes\"\n"
        done

        cat <<EOF > "${result_dir}/delta-deployment.yaml"
apiVersion: apps/v1
kind: Deployment
metadata:
  name: massive-delta-${iter_num}
  namespace: default
spec:
  replicas: $delta
  selector:
    matchLabels:
      delta-id: "${iter_num}"
  template:
    metadata:
      labels:
        delta-id: "${iter_num}"
$(echo -e "$dynamic_labels" | sed '/^$/d')
    spec:
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: type
                operator: In
                values:
                - kwok
      tolerations:
      - key: "kwok.x-k8s.io/node"
        operator: "Exists"
        effect: "NoSchedule"
      containers:
      - name: web
        image: nginx:alpine
        ports:
        - containerPort: 80
EOF
        kubectl apply -f "${result_dir}/delta-deployment.yaml"
        
        # 2. Wait for these specific delta pods to be Ready before unleashing them
        local timeout=3600
        local end_time=$(($(date +%s) + timeout))
        while [[ $(date +%s) -lt $end_time ]]; do
            local ready=$(kubectl get deploy massive-delta-${iter_num} -o jsonpath='{.status.readyReplicas}' 2>/dev/null || echo 0)
            ready=${ready:-0}
            echo "[$(date +%T)] Delta Pods Ready: $ready / $delta"
            if [[ "$ready" -ge "$delta" ]]; then break; fi
            sleep 5
        done
    fi

    echo -e "\n[$(date +%T)] PHASE 2: TRIGGERING DELTA TSUNAMI"
    
    # Ensure service exists, if not, create it pointing to nowhere
    if ! kubectl get service massive-scale-service >/dev/null 2>&1; then
        kubectl apply -f "${SERVICE_YAML}"
        # Set to dummy selector to prevent initialization trickle by completely replacing the selector via JSON patch
        kubectl patch service massive-scale-service --type=json -p='[{"op": "replace", "path": "/spec/selector", "value": {"match-0":"yes"}}]' 2>/dev/null || true
    fi
    
    # Start the clock EXACTLY before the selector switch
    local deploy_start=$(date +%s)
    echo "$deploy_start" > "${result_dir}/deploy_start_timestamp.txt"
    
    echo "[$(date +%T)] Instantly patching Service selector to 'match-${iter_num}: yes'..."
    # A single, instantaneous API call replacing the entire selector
    kubectl patch service massive-scale-service --type=json -p="[{\"op\": \"replace\", \"path\": \"/spec/selector\", \"value\": {\"match-${iter_num}\":\"yes\"}}]"
    
    local timeout=3600 # 60 mins hard safety
    local end_time=$(($(date +%s) + timeout))
    local interval=5
    
    echo -e "\n[$(date +%T)] Monitoring kube-proxy data-plane sync..."
    
    while [[ $(date +%s) -lt $end_time ]]; do
        local endpoints=$(kubectl get endpointslices -l kubernetes.io/service-name=massive-scale-service -o jsonpath='{.items[*].endpoints[*].addresses[*]}' 2>/dev/null | wc -w || echo "0")
        
        echo "[$(date +%T)] Endpoints Sync'd: ${endpoints}/${target_replicas}"
        
        if [[ "$endpoints" -ge "$target_replicas" ]]; then
            echo "[$(date +%T)] Target reached! Waiting 20 seconds for kube-proxy to finish writing rules and Prometheus to scrape..."
            sleep 20
            break
        fi
        
        sleep $interval
    done
    
    echo "[$(date +%T)] Waiting an additional 10 seconds for trailing metric collection..."
    sleep 10
    
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
    
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[15s])))' "${result_dir}/sync_duration_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.95, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[15s])))' "${result_dir}/sync_duration_p95_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[15s])))' "${result_dir}/sync_duration_p50_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[15s])))' "${result_dir}/network_programming_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.95, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[15s])))' "${result_dir}/network_programming_p95_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.50, sum by (le) (rate(kubeproxy_network_programming_duration_seconds_bucket[15s])))' "${result_dir}/network_programming_p50_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(rate(process_cpu_seconds_total{job="kube-proxy"}[15s]))' "${result_dir}/cpu_usage_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(process_resident_memory_bytes{job="kube-proxy"})' "${result_dir}/memory_usage_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'histogram_quantile(0.99, sum by (le) (rate(apiserver_request_duration_seconds_bucket{verb=~"POST|PUT|PATCH",resource=~"endpoints.*"}[15s])))' "${result_dir}/apiserver_latency_p99_timeseries.json" "$start_time" "$end_time" "$step"
    query_prometheus_range 'sum(rate(kubeproxy_sync_proxy_rules_duration_seconds_count[15s]))' "${result_dir}/sync_count_timeseries.json" "$start_time" "$end_time" "$step"
    
    # Final state metrics
    query_prometheus 'count(kube_pod_info{pod=~"massive-delta-.*"})' "${result_dir}/final_pod_count.json"
}

collect_cluster_state() {
    local result_dir="$1"
    echo -e "\n[$(date +%T)] Collecting cluster state..."
    
    kubectl get nodes -o wide > "${result_dir}/nodes.txt" 2>/dev/null || true
    
    # Count pods on KWOK fake nodes
    kubectl get nodes -l type=kwok -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | while read node; do
        [[ -n "$node" ]] && kubectl get pods --all-namespaces --field-selector spec.nodeName=$node --no-headers 2>/dev/null | wc -l || echo "0"
    done | awk '{sum+=$1} END {print sum}' > "${result_dir}/kwok_pods_count.txt"
    
    kubectl get deployments -l 'delta-id' -o yaml > "${result_dir}/deployment_state.yaml" 2>/dev/null || true
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
    echo "Iteration ${iter_num}/${total_iters}: Scaling from ${prev_replicas} to ${replicas} (Cumulative Service Patch)"
    echo "========================================"
    
    local iter_dir="${BASE_RESULT_DIR}/replicas_${replicas}"
    mkdir -p "$iter_dir"
    
    collect_baseline_metrics "$iter_dir"
    delta_tsunami_and_monitor "$replicas" "$prev_replicas" "$iter_num" "$total_iters" "$iter_dir"
    
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
    echo "Cumulative Service Patch Methodology:"
    echo "  - Uses boolean label logic to group old and new pods."
    echo "  - Delta pods are deployed and pre-warmed invisibly."
    echo "  - A single O(1) instantaneous kubectl patch is executed."
    echo "  - Captures exact delta load spikes on top of existing rules."
    echo "========================================"
}

main