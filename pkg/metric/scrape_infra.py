#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY, KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

import subprocess
import json
import sys
import os
import urllib.parse

# --- Configuration ---
prometheus_ip = os.popen("kubectl get svc -n monitoring | grep prometheus-kube-prometheus-prometheus | awk '{print $3}'").read().strip().split('\n')[0]

# --- Helper Functions ---

def get_promql_query(query):
    """
    Legacy function to get a single value column using the promql tool.
    Used for the original node metrics.
    """
    def promql_query():
        return f"tools/bin/promql --no-headers --host 'http://{prometheus_ip}:9090' '{query}' | grep . | LC_COLLATE=C sort | awk '{{print $2}}'"
    return promql_query

def get_json_from_prometheus(query):
    """
    Queries the Prometheus HTTP API directly using curl to get a reliable JSON response.
    """
    encoded_query = urllib.parse.quote(query)
    cmd = f"curl -s 'http://{prometheus_ip}:9090/api/v1/query?query={encoded_query}'"
    try:
        output = subprocess.check_output(cmd, shell=True, universal_newlines=True)
        return json.loads(output)
    except (subprocess.CalledProcessError, json.JSONDecodeError) as e:
        print(f"Error querying Prometheus for query '{query}': {e}", file=sys.stderr)
        return None

# --- Main Execution ---

if __name__ == "__main__":
    # --- Define Shell Commands and Queries ---
    
    # Bash scripts
    cmd_get_loader_pct = ['bash', 'scripts/metrics/get_loader_cpu_pct.sh']
    cmd_get_abs_vals = ['bash', 'scripts/metrics/get_node_stats_abs.sh']
    cmd_get_pcts = ['bash', 'scripts/metrics/get_node_stats_percent.sh']
    cmd_get_pod_abs_vals = ['bash', 'scripts/metrics/get_pod_stats_abs.sh']
    
    # Original Kubernetes metrics queries
    query_mem_req = 'sum(kube_pod_container_resource_requests{resource="memory"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_mem_lim = 'sum(kube_pod_container_resource_limits{resource="memory"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_cpu_req = 'sum(kube_pod_container_resource_requests{resource="cpu"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_cpu_lim = 'sum(kube_pod_container_resource_limits{resource="cpu"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_pod_count = 'count(kube_pod_info and on(pod) max(kube_pod_container_status_running==1) by (pod)) by(node)'

    # Custom hardware_manager metrics queries (with job filter)
    query_cgroup_metrics = '{__name__=~"cgroup_core_count|cgroup_core_freq_setting|cgroup_avg_cpu_utilization|cgroup_avg_actual_core_freq", job="hardware_manager"}'
    query_cgroup_cpu_time = 'cgroup_cpu_time{job="hardware_manager"}'
    query_cgroup_cpu_pressure = 'cgroup_cpu_pressure{job="hardware_manager"}'
    query_node_power = 'node_power_consumption{job="hardware_manager"}'

    # --- Initialize Result Structure ---
    
    result = {
        "master_cpu_pct": 0,
        "master_cpu_req": 0,
        "master_cpu_lim": 0,
        "master_mem_pct": 0,
        "master_mem_req": 0,
        "master_mem_lim": 0,
        "master_pods": 0,
        "cpu": [],
        "cpu_req": [],
        "cpu_lim": [],
        "cpu_pct_avg": 0,
        "cpu_pct_active_avg": 0,
        "cpu_pct_max": 0,
        "memory": [],
        "memory_req": [],
        "memory_lim": [],
        "memory_pct": 0,
        "pod_cpu": [],
        "pod_mem": [],
        "pods": [],
        "loader_cpu": 0,
        "loader_mem": 0,
        "hardware_metrics": {},
    }

    # --- Fetch and Process Metrics ---

    # 1. Fetch Loader and original Kubernetes metrics
    result["loader_cpu"], result["loader_mem"] = list(
        map(float, subprocess.check_output(cmd_get_loader_pct).decode("utf-8").strip().split())
    )

    abs_out = subprocess.check_output(cmd_get_abs_vals).decode("utf-8")[:-1].split('\n')
    pcts_out = subprocess.check_output(cmd_get_pcts).decode("utf-8").split('\n')
    mem_req = os.popen(get_promql_query(query_mem_req)()).read().strip().split('\n')
    mem_lim = os.popen(get_promql_query(query_mem_lim)()).read().strip().split('\n')
    cpu_req = os.popen(get_promql_query(query_cpu_req)()).read().strip().split('\n')
    cpu_lim = os.popen(get_promql_query(query_cpu_lim)()).read().strip().split('\n')
    pod_count = os.popen(get_promql_query(query_pod_count)()).read().strip().split('\n')

    # 2. Fetch custom hardware_manager metrics via JSON API
    cgroup_metrics_data = get_json_from_prometheus(query_cgroup_metrics)
    cgroup_cpu_time_data = get_json_from_prometheus(query_cgroup_cpu_time)
    cgroup_cpu_pressure_data = get_json_from_prometheus(query_cgroup_cpu_pressure)
    node_power_data = get_json_from_prometheus(query_node_power)

    # 3. Parse custom hardware_manager metrics from JSON
    if cgroup_metrics_data and cgroup_metrics_data.get('status') == 'success':
        for res in cgroup_metrics_data['data']['result']:
            labels = res['metric']
            node = labels.get('node', '').strip()
            cgroup = labels.get('cgroup_name')
            if not node or not cgroup: continue
            
            result['hardware_metrics'].setdefault(node, {}).setdefault('cgroups', {}).setdefault(cgroup, {})
            metric_name = labels.get('__name__', '').replace('cgroup_', '')
            if metric_name:
                result['hardware_metrics'][node]['cgroups'][cgroup][metric_name] = float(res['value'][1])

    if cgroup_cpu_time_data and cgroup_cpu_time_data.get('status') == 'success':
        for res in cgroup_cpu_time_data['data']['result']:
            labels = res['metric']
            node = labels.get('node', '').strip()
            cgroup = labels.get('cgroup_name')
            type_label = labels.get('type')
            if not node or not cgroup or not type_label: continue
            
            result['hardware_metrics'].setdefault(node, {}).setdefault('cgroups', {}).setdefault(cgroup, {})
            key = f"cpu_time_{type_label}_us"
            result['hardware_metrics'][node]['cgroups'][cgroup][key] = float(res['value'][1])
            
    if cgroup_cpu_pressure_data and cgroup_cpu_pressure_data.get('status') == 'success':
        for res in cgroup_cpu_pressure_data['data']['result']:
            labels = res['metric']
            node = labels.get('node', '').strip()
            cgroup = labels.get('cgroup_name')
            type_label = labels.get('type')
            if not node or not cgroup or not type_label: continue
            
            result['hardware_metrics'].setdefault(node, {}).setdefault('cgroups', {}).setdefault(cgroup, {})
            key = f"cpu_pressure_{type_label}"
            result['hardware_metrics'][node]['cgroups'][cgroup][key] = float(res['value'][1])

    if node_power_data and node_power_data.get('status') == 'success':
        for res in node_power_data['data']['result']:
            labels = res['metric']
            node = labels.get('node', '').strip()
            if not node: continue
            
            result['hardware_metrics'].setdefault(node, {})
            result['hardware_metrics'][node]['power_consumption_watts'] = float(res['value'][1])

    # 4. Parse original Kubernetes metrics
    cpus = []
    mems = []
    counter = 0
    is_master = True
    for abs_vals, pcts, mem_r, mem_l, cpu_r, cpu_l, pod in zip(abs_out, pcts_out, mem_req, mem_lim, cpu_req, cpu_lim, pod_count):
        if is_master:
            # Record master node.
            result['master_cpu_pct'], result['master_mem_pct'] = list(map(float, pcts[:-1].split('%')))
            # FIX: Add checks for empty strings before converting to float/int
            result['master_mem_req'] = float(mem_r) if mem_r else 0.0
            result['master_mem_lim'] = float(mem_l) if mem_l else 0.0
            result['master_cpu_req'] = float(cpu_r) if cpu_r else 0.0
            result['master_cpu_lim'] = float(cpu_l) if cpu_l else 0.0
            result['master_pods'] = int(pod) if pod else 0
            is_master = False
            continue

        counter += 1
        cpu, mem = abs_vals.split(',')
        cpu_pct_avg, mem_pct = pcts[:-1].split('%')

        result['cpu'].append(cpu[1:-1])
        cpus.append(float(cpu_pct_avg))
        result['memory'].append(mem[1:-1])
        mems.append(float(mem_pct))
        # FIX: Add checks for empty strings before converting to float/int
        result['memory_req'].append(float(mem_r) if mem_r else 0.0)
        result['memory_lim'].append(float(mem_l) if mem_l else 0.0)
        result['cpu_req'].append(float(cpu_r) if cpu_r else 0.0)
        result['cpu_lim'].append(float(cpu_l) if cpu_l else 0.0)
        result['pods'].append(int(pod) if pod else 0)
    
    # 5. Calculate final aggregate values
    if counter != 0:
        result['cpu_pct_avg'] =  sum(cpus) / len(cpus)
        result['cpu_pct_max'] =  max(cpus)

        active_node = 0
        active_cpu = 0
        active_mem = 0
        for cpu, mem in zip(cpus, mems):
            if cpu >= 5: # Active node
                active_cpu += cpu
                active_mem += mem
                active_node += 1

        if not active_node: active_node = 1
        result['cpu_pct_active_avg'] = active_cpu / active_node
        result['memory_pct'] = active_mem / active_node

    else:
        result['cpu'] = ['']
        result['memory'] = ['']
    
    # --- Print Final Result ---
    
    print(json.dumps(result, indent=4))