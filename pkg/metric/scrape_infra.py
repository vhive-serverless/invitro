import subprocess

if __name__ == "__main__":
    cmd_get_abs_vals = ['bash', 'scripts/metrics/get_node_stats_abs.sh']
    cmd_get_pcts = ['bash', 'scripts/metrics/get_node_stats_percent.sh']

    abs_out = subprocess.check_output(cmd_get_abs_vals).decode("utf-8")[:-1]
    pcts_out = subprocess.check_output(cmd_get_pcts).decode("utf-8")

    result = {
        "cpu": [],
        "cpu_pct": 0,
        "memory": [],
        "memory_pct": 0,
    }
    counter = 0
    for abs_vals, pcts in zip(abs_out.split('\n'), pcts_out.split('\n')):
        counter += 1
        cpu, mem = abs_vals.split(',')
        cpu_pct, mem_pct = pcts[:-1].split('%')
        result = result.copy()
        result['cpu'].append(cpu[1:-1])
        result['cpu_pct'] += int(cpu_pct)
        result['memory'].append(mem[1:-1])
        result['memory_pct'] += int(mem_pct)
    result['cpu_pct'] /= counter
    result['memory_pct'] /= counter
    
    print(result)
