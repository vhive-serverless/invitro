# Kube-Proxy Mode Comparison Experiment

This directory contains automation scripts to compare control plane latency between **iptables** and **nftables** modes for kube-proxy using a **Two-Step Tsunami Architecture**.

## Overview

The experiment uses a two-phase approach to isolate kube-proxy performance from Kubernetes API bottlenecks:
1. **Phase 1 (Pre-warming):** Deploy thousands of pods and wait for them to be Ready (bypassing API rate limits)
2. **Phase 2 (Tsunami):** Apply a Service to instantly group all pods, forcing kube-proxy to process all endpoints simultaneously

This methodology measures:
- Proxy rules sync duration (p50, p95, p99)
- Network programming latency (end-to-end)
- CPU and memory consumption during the tsunami
- API server latency for EndpointSlice creation

## Prerequisites

1. **Cluster Setup**
   - KWOK fake nodes deployed (`ENABLE_KWOK=true` in setup.cfg)
   - At least 10 KWOK nodes with 50k pod capacity each
   - Prometheus deployed (`DEPLOY_PROMETHEUS=true`)

2. **Tools Required**
   - `kubectl` configured for your cluster
   - `curl` for Prometheus API queries
   - Bash shell (Linux/WSL/Git Bash)
   - `sudo` access (for control plane turbocharging)

3. **Prometheus Access**
   - Port-forward Prometheus if not externally accessible:
     ```bash
     kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
     ```

## Important: Turbocharge Control Plane (One-Time Setup)

**Before running large-scale experiments (10k+ replicas), you MUST increase Kubernetes API rate limits** to prevent the control plane from becoming a bottleneck during pod pre-warming.

Run this script once on your control plane node:

```bash
cd experiments/proxy_mode_comparison
chmod +x turbocharge-control-plane.sh
./turbocharge-control-plane.sh
```

This script will:
- Backup existing control plane manifests
- Increase API server request limits (500 mutating, 1000 total)
- Increase controller-manager QPS (100) and concurrency (50 workers)
- Increase scheduler QPS (100)
- Increase kube-proxy client QPS (100)
- Automatically restart all affected components

**⚠️ Wait 2-3 minutes after running this script before starting experiments.**

## Quick Start

### 0. Turbocharge Control Plane (Required for Large-Scale Tests)

```bash
# Run once before experiments
chmod +x turbocharge-control-plane.sh
./turbocharge-control-plane.sh

# Wait 2-3 minutes for components to restart
sleep 180
```

### 1. Prepare YAML Files

The experiment requires two separate YAML files:
- `deployment-only.yaml` - Pod deployment without Service
- `service-only.yaml` - Service definition only

These files enable the Two-Step Tsunami architecture.

### 2. Run Experiment with iptables

```bash
# Ensure kube-proxy is in iptables mode
kubectl -n kube-system edit cm kube-proxy
# Set mode: "iptables" in config, then restart kube-proxy pods
kubectl -n kube-system rollout restart ds kube-proxy
kubectl -n kube-system rollout status ds kube-proxy

# Run the experiment (tests multiple replica counts)
cd experiments/proxy_mode_comparison
chmod +x run_experiment.sh
./run_experiment.sh --mode iptables
```

### 3. Switch to nftables and Re-run

```bash
# Switch kube-proxy to nftables mode
kubectl -n kube-system edit cm kube-proxy
# Set mode: "nftables" in config, then restart kube-proxy pods
kubectl -n kube-system rollout restart ds kube-proxy
kubectl -n kube-system rollout status ds kube-proxy

# Run the experiment again
./run_experiment.sh --mode nftables
```

### 4. Compare Results

Results are saved in `./results/<mode>_<timestamp>/replicas_<count>/`:
- `SUMMARY.md` - Experiment summary
- `*_timeseries.json` - Metric data over time (Phase 2 only)
- `baseline_*.json` - Pre-tsunami metrics
- `deploy_start_timestamp.txt` - Service creation time (T=0)
- `deploy_end_timestamp.txt` - kube-proxy sync completion time
- Various cluster state files

## Script Options

```bash
./run_experiment.sh [OPTIONS]

Required:
  --mode MODE           Current kube-proxy mode (iptables or nftables)

Optional:
  --prometheus-url URL  Prometheus URL (default: http://localhost:9090)
  --output-dir DIR      Results directory (default: ./results)
  --cleanup-wait SEC    Seconds to wait between iterations (default: 60)
```

The script automatically iterates through: **100, 500, 1000, 5000, 10000, 20000, 30000 replicas**

### Examples

**Basic run:**
```bash
./run_experiment.sh --mode iptables
```

**Custom Prometheus endpoint:**
```bash
./run_experiment.sh --mode nftables --prometheus-url http://192.168.1.100:9090
```

**Custom cleanup wait:**
```bash
./run_experiment.sh --mode iptables --cleanup-wait 120
```

## How the Two-Step Tsunami Works

### Traditional Approach (Problematic)
- Deploy pods + service simultaneously
- K8s API slowly assigns IPs over 15-30 minutes
- kube-proxy processes endpoints in small batches
- **Result:** Impossible to measure true kube-proxy performance

### Two-Step Tsunami (This Experiment)
1. **Phase 1 - Pre-warm:** Deploy 30k pods, wait until all are Ready
   - Takes 15-30 minutes (not measured)
   - kube-proxy is idle (no Service exists yet)
   
2. **Phase 2 - Tsunami:** Apply Service
   - **T=0:** Service created, timestamp recorded
   - EndpointSlice controller instantly batches all 30k IPs
   - kube-proxy receives massive update, starts syncing
   - Script monitors sync completion via Prometheus
   - **T=end:** Sync rate hits 0, timestamp recorded
   
3. **Metrics Extraction:** Query Prometheus using T=0 to T=end
   - Graphs show pure kube-proxy performance
   - API rate limits completely bypassed

**Key Insight:** This isolates kernel-level routing performance from control plane scheduling delays.

**Custom Prometheus URL:**
```bash
./run_experiment.sh --mode nftables --prometheus-url http://prometheus.example.com:9090
```

## Metric Retrieval Options

### Option 1: Automated Script (Recommended)

The `run_experiment.sh` script automatically collects all metrics via Prometheus API and saves JSON files.

**Pros:**
- Fully automated
- Timestamped results
- Easy comparison between modes
- Reproducible

**Cons:**
- Requires Prometheus API access
- JSON format needs post-processing for visualization

### Option 2: Grafana Dashboards

Import or create dashboards to visualize metrics in real-time.

**Setup:**
1. Port-forward Grafana:
   ```bash
   kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
   ```

2. Access at `http://localhost:3000` (default: admin/prom-operator)

3. Create a dashboard with panels for:
   - `histogram_quantile(0.99, rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m]))`
   - `histogram_quantile(0.99, rate(kubeproxy_network_programming_duration_seconds_bucket[1m]))`
   - `sum(rate(process_cpu_seconds_total{job="kube-proxy"}[1m]))`
   - `sum(process_resident_memory_bytes{job="kube-proxy"})`

4. Set dashboard refresh to 5s, enable time range picker

**Pros:**
- Real-time visualization
- Built-in graphing
- Easy to spot trends
- Can export data/screenshots

**Cons:**
- Manual dashboard creation
- Need to export data separately for reports

### Option 3: Direct Prometheus Queries

Query Prometheus directly via UI or API for ad-hoc analysis.

**Via Web UI:**
1. Access Prometheus: `http://localhost:9090`
2. Go to Graph tab
3. Enter queries (see below)
4. Adjust time range

**Via API (curl):**
```bash
# Current value
curl -G 'http://localhost:9090/api/v1/query' \
  --data-urlencode 'query=histogram_quantile(0.99, rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m]))'

# Time range
START=$(date -d '5 minutes ago' +%s)
END=$(date +%s)
curl -G 'http://localhost:9090/api/v1/query_range' \
  --data-urlencode 'query=histogram_quantile(0.99, rate(kubeproxy_sync_proxy_rules_duration_seconds_bucket[1m]))' \
  --data-urlencode "start=$START" \
  --data-urlencode "end=$END" \
  --data-urlencode 'step=5'
```

**Pros:**
- Quick spot checks
- No automation needed
- Flexible queries

**Cons:**
- Manual process
- Hard to compare across time periods
- No persistent storage

### Option 4: Export via Prometheus API to CSV

Convert JSON results to CSV for analysis in Excel/Python/R.

**Python example:**
```python
import json
import csv

with open('results/iptables_20260304/sync_duration_p99_timeseries.json') as f:
    data = json.load(f)

with open('sync_duration.csv', 'w', newline='') as csvfile:
    writer = csv.writer(csvfile)
    writer.writerow(['timestamp', 'value'])
    
    for result in data['data']['result']:
        for value in result['values']:
            writer.writerow([value[0], value[1]])
```

### Option 5: kubectl + jq for Quick Checks

Check metrics directly from kube-proxy pods (if exposed):

```bash
# If kube-proxy exposes metrics on :10249
kubectl get pods -n kube-system -l k8s-app=kube-proxy -o wide
POD=$(kubectl get pods -n kube-system -l k8s-app=kube-proxy -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n kube-system $POD -- curl -s localhost:10249/metrics | grep kubeproxy_sync
```

**Pros:**
- No Prometheus needed
- Direct access

**Cons:**
- Point-in-time only
- No historical data
- Manual aggregation needed

## Key Metrics Explained

| Metric | Description | Why It Matters |
|--------|-------------|----------------|
| `kubeproxy_sync_proxy_rules_duration_seconds` | Time to sync iptables/nftables rules | Core latency metric - shows how long it takes to update network rules |
| `kubeproxy_network_programming_duration_seconds` | End-to-end network programming time | Total time from endpoint change to rules applied |
| `process_cpu_seconds_total` | CPU consumption | Resource efficiency comparison |
| `process_resident_memory_bytes` | Memory usage | Memory efficiency comparison |
| `apiserver_request_duration_seconds` | API server response time | Shows if proxy mode impacts API load |
| `kubeproxy_sync_proxy_rules_duration_seconds_count` | Sync frequency | How often rules are synchronized |

## Analysis Tips

1. **Focus on percentiles**: p99 shows worst-case latency, p50 shows typical
2. **Watch for spikes**: Large spikes indicate performance issues
3. **CPU correlation**: Higher CPU might indicate less efficient rule processing
4. **Compare baseline**: Check metrics before deployment vs. during
5. **Scale matters**: Test with different replica counts (5k, 10k, 20k)

## Expected Results

**Hypothesis**: nftables should show:
- Lower sync duration (more efficient rule updates)
- Lower CPU usage (better rule processing)
- Better scaling characteristics (less degradation at high endpoint count)

**What to watch for:**
- Sync latency spikes when 5000 endpoints are added
- Steady-state performance after initial burst
- Memory usage patterns
- API server load differences

## Troubleshooting

**Prometheus not accessible:**
```bash
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
```

**Kube-proxy mode not changing:**
```bash
# Verify config
kubectl -n kube-system get cm kube-proxy -o yaml | grep mode

# Force restart
kubectl -n kube-system delete pod -l k8s-app=kube-proxy

# Check logs
kubectl -n kube-system logs -l k8s-app=kube-proxy --tail=50
```

**Pods not deploying to KWOK nodes:**
```bash
# Check KWOK nodes exist
kubectl get nodes -l type=kwok

# Check node taints
kubectl describe node kwok-node-0 | grep Taints

# Verify tolerations in deployment
kubectl get deployment massive-scale-deployment -o yaml | grep -A5 tolerations
```

**No metrics returned:**
```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets | jq '.data.activeTargets[] | select(.labels.job=="kube-proxy")'

# Check if kube-proxy is scraped
curl 'http://localhost:9090/api/v1/query?query=up{job="kube-proxy"}'
```

## Cleanup

**After experiment:**
```bash
# Delete the massive deployment
kubectl delete deployment massive-scale-deployment
kubectl delete service massive-scale-service

# Or use the --cleanup flag
./run_experiment.sh --mode iptables --cleanup
```

**Reset cluster:**
```bash
# If you want to start fresh
kubectl delete deployment massive-scale-deployment
kubectl delete service massive-scale-service
kubectl delete pods -l app=fake-workload --force --grace-period=0
```

## Further Reading

- [Kubernetes kube-proxy modes](https://kubernetes.io/docs/reference/networking/virtual-ips/#proxy-modes)
- [nftables vs iptables performance](https://wiki.nftables.org/wiki-nftables/index.php/Main_differences_with_iptables)
- [Prometheus query examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
