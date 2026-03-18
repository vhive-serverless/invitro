import os
import json
import argparse
import matplotlib.pyplot as plt
import numpy as np

def parse_args():
    parser = argparse.ArgumentParser(description="Generate plots proving kube-proxy batching behavior.")
    parser.add_argument('--results-dir', required=True, help='Path to the base results directory (e.g., ./results_batch)')
    parser.add_argument('--output-dir', required=True, help='Directory to save the generated graphs')
    return parser.parse_args()

def load_data(filepath):
    if not os.path.exists(filepath):
        return None, None
    with open(filepath, 'r') as f:
        data = json.load(f)
        try:
            res = data['data']['result'][0]['values']
            times = []
            vals = []
            for v in res:
                if v[1] != 'NaN':
                    times.append(float(v[0]))
                    vals.append(float(v[1]))
            if times:
                # Normalize time to start at 0
                return np.array(times) - times[0], np.array(vals)
        except (KeyError, IndexError):
            pass
    return None, None

def plot_batch_experiment(results_dir, output_dir):
    os.makedirs(output_dir, exist_ok=True)
    
    # Process all subdirectories in the results directory (like iptables_2026... and nftables_2026...)
    try:
        run_dirs = [d for d in os.listdir(results_dir) if os.path.isdir(os.path.join(results_dir, d))]
    except FileNotFoundError:
        print(f"Error: Directory {results_dir} not found.")
        return

    graphs_generated = 0

    for run_dir in run_dirs:
        # Extract mode from the folder prefix (iptables_xxx or nftables_xxx)
        mode = run_dir.split('_')[0]
        base_path = os.path.join(results_dir, run_dir)
        
        trickle_dir = os.path.join(base_path, 'replicas_5000_trickle')
        bulk_dir = os.path.join(base_path, 'replicas_5000_bulk')
        
        if not os.path.exists(trickle_dir) or not os.path.exists(bulk_dir):
            continue
            
        # Ensure we have data
        tt_t, tv_t = load_data(os.path.join(trickle_dir, 'sync_count_timeseries.json'))
        tt_b, tv_b = load_data(os.path.join(bulk_dir, 'sync_count_timeseries.json'))
        
        if tt_t is None or tt_b is None:
            print(f"No valid timeseries data found in {run_dir}. Skipping.")
            continue
            
        fig, axes = plt.subplots(1, 3, figsize=(18, 5))
        ax1, ax2, ax3 = axes
        
        # ---------------------------------------------------------
        # PLOT 1: Sync Rate Over Time
        # ---------------------------------------------------------
        ax1.plot(tt_t, tv_t, label='Trickle (100 pods, 1/sec)', color='darkorange', linewidth=2)
        ax1.plot(tt_b, tv_b, label='Bulk (100 pods instantly)', color='steelblue', linewidth=2)
        ax1.set_title(f'Kube-Proxy Sync Rate over Time ({mode.capitalize()})', fontsize=12, fontweight='bold')
        ax1.set_xlabel('Time (seconds)', fontsize=11)
        ax1.set_ylabel('Sync Operations Per Second', fontsize=11)
        ax1.legend()
        ax1.grid(True, linestyle='--', alpha=0.7)
        
        # ---------------------------------------------------------
        # PLOT 2: Total Estimated Syncs (Integrated Area)
        # ---------------------------------------------------------
        # Since step=1s, summing the rates over the duration gives a close approximation of total operations
        trickle_total = np.sum(tv_t)
        bulk_total = np.sum(tv_b)
        
        bars = ax2.bar(['Trickle\n(Sequential)', 'Bulk\n(Batched)'], [trickle_total, bulk_total], color=['darkorange', 'steelblue'], width=0.6)
        ax2.set_title(f'Total Kube-Proxy Sync Cycles ({mode.capitalize()})', fontsize=12, fontweight='bold')
        ax2.set_ylabel('Total Syncs (Area under curve)', fontsize=11)
        
        # Anchor floating text above max limits to make it visually clear
        for bar in bars:
            height = bar.get_height()
            ax2.text(bar.get_x() + bar.get_width()/2., height + (max(trickle_total, bulk_total)*0.02),
                     f'{height:.1f}', ha='center', va='bottom', fontweight='bold', fontsize=11)
        ax2.set_ylim(0, max(trickle_total, bulk_total) * 1.15)
        
        # ---------------------------------------------------------
        # PLOT 3: Kube-Proxy CPU Overhead
        # ---------------------------------------------------------
        cpu_t_t, cpu_v_t = load_data(os.path.join(trickle_dir, 'cpu_usage_timeseries.json'))
        cpu_t_b, cpu_v_b = load_data(os.path.join(bulk_dir, 'cpu_usage_timeseries.json'))
        
        if cpu_t_t is not None and cpu_t_b is not None:
            ax3.plot(cpu_t_t, cpu_v_t, label='Trickle CPU Profile', color='darkorange', linewidth=2)
            ax3.plot(cpu_t_b, cpu_v_b, label='Bulk CPU Profile', color='steelblue', linewidth=2)
            ax3.set_title(f'Kube-Proxy CPU Overhead ({mode.capitalize()})', fontsize=12, fontweight='bold')
            ax3.set_xlabel('Time (seconds)', fontsize=11)
            ax3.set_ylabel('CPU Usage (Cores)', fontsize=11)
            ax3.legend()
            ax3.grid(True, linestyle='--', alpha=0.7)
        else:
            ax3.text(0.5, 0.5, "CPU Timeseries Missing", ha='center', va='center')
        
        # Adjust layout and save
        plt.tight_layout()
        out_filename = os.path.join(output_dir, f'batching_proof_{mode}_{run_dir.split("_")[1]}.png')
        plt.savefig(out_filename, dpi=300, bbox_inches='tight')
        plt.close()
        print(f"Generated Plot: {out_filename}")
        graphs_generated += 1

    if graphs_generated == 0:
        print("No valid batch experiment data found. Ensure you point '--results-dir' to the directory containing 'iptables_...' or 'nftables_...' folders.")

if __name__ == '__main__':
    args = parse_args()
    plot_batch_experiment(args.results_dir, args.output_dir)