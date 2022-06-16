import subprocess
import json
import sys

loader_total_cores = 8

if __name__ == "__main__":
    cmd_get_loader_pct = ['bash', 'scripts/metrics/get_loader_cpu_pct.sh']
    cmd_get_abs_vals = ['bash', 'scripts/metrics/get_node_stats_abs.sh']
    cmd_get_pcts = ['bash', 'scripts/metrics/get_node_stats_percent.sh']

    result = {
        "master_cpu_pct": 0,
        "master_mem_pct": 0,
        "cpu": [],
        "cpu_pct_avg": 0,
        "cpu_pct_active_avg": 0,
        "cpu_pct_max": 0,
        "memory": [],
        "memory_pct": 0,
    }

    loader_cpu_pct, loader_mem_pct = list(
        map(float, subprocess.check_output(cmd_get_loader_pct).decode("utf-8").strip().split())
    )
    loader_cpu_pct /= loader_total_cores #* Normalise to [0, 100]

    abs_out = subprocess.check_output(cmd_get_abs_vals).decode("utf-8")[:-1]
    pcts_out = subprocess.check_output(cmd_get_pcts).decode("utf-8")

    cpus = []
    mems = []
    counter = 0
    is_master = True
    for abs_vals, pcts in zip(abs_out.split('\n'), pcts_out.split('\n')):
        if is_master:
            # Record master node.
            result['master_cpu_pct'], result['master_mem_pct'] = list(map(float, pcts[:-1].split('%')))
            result['master_cpu_pct'] = max(0, result['master_cpu_pct'] - loader_cpu_pct)
            result['master_mem_pct'] = max(0, result['master_mem_pct'] - loader_mem_pct)
            is_master = False
            continue

        counter += 1
        cpu, mem = abs_vals.split(',')
        cpu_pct_avg, mem_pct = pcts[:-1].split('%')

        result['cpu'].append(cpu[1:-1])
        cpus.append(float(cpu_pct_avg))
        result['memory'].append(mem[1:-1])
        mems.append(float(mem_pct))
    
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
