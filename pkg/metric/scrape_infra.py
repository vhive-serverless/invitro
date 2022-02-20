from re import sub
import subprocess

if __name__ == "__main__": 
    cmd_get_abs_vals = ['bash', 'scripts/metrics/get_node_stats_abs.sh']
    cmd_get_pcts = ['bash', 'scripts/metrics/get_node_stats_percent.sh']
    
    abs_out = subprocess.check_output(cmd_get_abs_vals).decode("utf-8")
    pcts_out = subprocess.check_output(cmd_get_pcts).decode("utf-8")

    print(abs_out, pcts_out)

    model = {}
    for abs_vals, pcts in zip(abs_out.split('\n'), pcts_out.split('\n')):
        cpu, mem = abs_vals.split(',')
        cpu_pct, mem_pct = pcts[:-1].split('%')
        m = model.copy()
        m['cpu'] = cpu[1:-1]
        m['cpu_pct'] = cpu_pct
        m['memory'] = mem[1:-1]
        m['memory_pct'] = mem_pct

    print(model)