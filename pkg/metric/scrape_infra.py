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
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
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

prometheus_ip = os.popen("kubectl get svc -n monitoring | grep prometheus-kube-prometheus-prometheus | awk '{print $3}'").read().strip().split('\n')[0]

def get_promql_query(query):
    def promql_query():
        return "tools/bin/promql --no-headers --host 'http://" + prometheus_ip + ":9090' '" + query + "' | grep . | LC_COLLATE=C sort | awk '{print $2}'"
    return promql_query

if __name__ == "__main__":
    cmd_get_loader_pct = ['bash', 'scripts/metrics/get_loader_cpu_pct.sh']
    cmd_get_abs_vals = ['bash', 'scripts/metrics/get_node_stats_abs.sh']
    cmd_get_pcts = ['bash', 'scripts/metrics/get_node_stats_percent.sh']
    cmd_get_pod_abs_vals = ['bash', 'scripts/metrics/get_pod_stats_abs.sh']
    query_mem_req = 'sum(kube_pod_container_resource_requests{resource="memory"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_mem_lim = 'sum(kube_pod_container_resource_limits{resource="memory"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_cpu_req = 'sum(kube_pod_container_resource_requests{resource="cpu"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_cpu_lim = 'sum(kube_pod_container_resource_limits{resource="cpu"} and on(container, pod) (kube_pod_container_status_running==1) or on(node) (kube_node_info*0)) by (node)'
    query_pod_count = 'count(kube_pod_info and on(pod) max(kube_pod_container_status_running==1) by (pod)) by(node)'

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
    }

    result["loader_cpu"], result["loader_mem"] = list(
        map(float, subprocess.check_output(cmd_get_loader_pct).decode("utf-8").strip().split())
    )

    abs_out = subprocess.check_output(cmd_get_abs_vals).decode("utf-8")[:-1].split('\n')
    pcts_out = subprocess.check_output(cmd_get_pcts).decode("utf-8").split('\n')
    # pod_abs_out = subprocess.check_output(cmd_get_pod_abs_vals).decode("utf-8")[:-1].split('\n')
    mem_req = os.popen(get_promql_query(query_mem_req)()).read().strip().split('\n')
    mem_lim = os.popen(get_promql_query(query_mem_lim)()).read().strip().split('\n')
    cpu_req = os.popen(get_promql_query(query_cpu_req)()).read().strip().split('\n')
    cpu_lim = os.popen(get_promql_query(query_cpu_lim)()).read().strip().split('\n')
    pod_count = os.popen(get_promql_query(query_pod_count)()).read().strip().split('\n')

    cpus = []
    mems = []
    counter = 0
    is_master = True
    for abs_vals, pcts, mem_r, mem_l, cpu_r, cpu_l, pod in zip(abs_out, pcts_out, mem_req, mem_lim, cpu_req, cpu_lim, pod_count):
        if is_master:
            # Record master node.
            result['master_cpu_pct'], result['master_mem_pct'] = list(map(float, pcts[:-1].split('%')))
            result['master_mem_req'] = float(mem_r)
            result['master_mem_lim'] = float(mem_l)
            result['master_cpu_req'] = float(cpu_r)
            result['master_cpu_lim'] = float(cpu_l)
            result['master_pods'] = int(pod)
            is_master = False
            continue

        counter += 1
        cpu, mem = abs_vals.split(',')
        cpu_pct_avg, mem_pct = pcts[:-1].split('%')

        result['cpu'].append(cpu[1:-1])
        cpus.append(float(cpu_pct_avg))
        result['memory'].append(mem[1:-1])
        mems.append(float(mem_pct))
        result['memory_req'].append(float(mem_r))
        result['memory_lim'].append(float(mem_l))
        result['cpu_req'].append(float(cpu_r))
        result['cpu_lim'].append(float(cpu_l))
        result['pods'].append(int(pod))
    

    # for pod_abs_vals in pod_abs_out:
    #     pod_cpu, pod_mem = pod_abs_vals.split(' ')
    #     result['pod_cpu'].append(pod_cpu)
    #     result['pod_mem'].append(pod_mem)
    
    # Prevent div-0 in the case of single-node.
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
        result['memory_pct'] = active_mem / active_node # Active memory average.

    else:
        result['cpu'] = ['']
        result['memory'] = ['']

    print(json.dumps(result))
