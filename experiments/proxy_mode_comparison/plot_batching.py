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

    # Data structures to extract results across multiple modes
    # e.g., {'iptables': {'trickle': X, 'bulk': Y}, 'nftables': {'trickle': A, 'bulk': B}}
    metrics_map = {}

    for run_dir in run_dirs:
        mode = run_dir.split('_')[0].lower() # 'iptables' or 'nftables'
        if mode not in ['iptables', 'nftables']:
            continue
            
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
            
        # Calculate integrals (sums via 1s steps)
        trickle_total = np.sum(tv_t)
        bulk_total = np.sum(tv_b)
        
        if mode not in metrics_map:
            metrics_map[mode] = {'trickle': trickle_total, 'bulk': bulk_total}

    if not metrics_map:
        print("No valid batch experiment data found for plotting.")
        return
        
    # Build Combined Side-by-Side Bar Chart
    labels = []
    trickle_vals = []
    bulk_vals = []
    
    # Ensure standard order (iptables then nftables if both exist)
    modes_found = sorted(metrics_map.keys())
    
    for m in modes_found:
        labels.append(m.capitalize())
        trickle_vals.append(metrics_map[m]['trickle'])
        bulk_vals.append(metrics_map[m]['bulk'])

    x = np.arange(len(labels))
    width = 0.35

    fig, ax = plt.subplots(figsize=(8, 6))
    
    # Create grouped bars
    rects1 = ax.bar(x - width/2, trickle_vals, width, label='Incremental Trickle (1 by 1)', color='darkorange')
    rects2 = ax.bar(x + width/2, bulk_vals, width, label='Instant Bulk (All 100 instantly)', color='steelblue')

    # Formatting properties
    ax.set_ylabel('Total Kube-Proxy Sync Cycles', fontsize=12)
    ax.set_title('Impact of endpoint batching on kube-proxy sync instances', fontsize=14, fontweight='bold')
    ax.set_xticks(x)
    ax.set_xticklabels(labels, fontsize=12)
    ax.legend(fontsize=11)
    ax.grid(axis='y', linestyle='--', alpha=0.7)
    
    # Auto-label above bars
    def autolabel(rects):
        for rect in rects:
            height = rect.get_height()
            ax.annotate(f'{height:.0f}',
                        xy=(rect.get_x() + rect.get_width() / 2, height),
                        xytext=(0, 3),  # 3 points vertical offset
                        textcoords="offset points",
                        ha='center', va='bottom', fontweight='bold', fontsize=11)

    autolabel(rects1)
    autolabel(rects2)

    # Scale Y slightly so text doesn't hit ceiling
    max_height = max(max(trickle_vals), max(bulk_vals)) if labels else 1
    ax.set_ylim(0, max_height * 1.15)
    
    plt.tight_layout()
    out_filename = os.path.join(output_dir, 'combined_batching_proof.png')
    plt.savefig(out_filename, dpi=300, bbox_inches='tight')
    plt.close()
    
    print(f"✅ Generated combined batching graph at: {out_filename}")

if __name__ == '__main__':
    args = parse_args()
    plot_batch_experiment(args.results_dir, args.output_dir)