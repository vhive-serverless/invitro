import json
import json
import os
import glob
import matplotlib.pyplot as plt
import argparse
from datetime import datetime

# Configure plot styling
plt.style.use('seaborn-v0_8-whitegrid')
COLORS = {'iptables': '#1f77b4', 'nftables': '#ff7f0e'}

def load_timeseries_data(filepath):
    """Load and extract values from Prometheus time-series JSON"""
    if not os.path.exists(filepath):
        return []
    
    with open(filepath, 'r') as f:
        try:
            data = json.load(f)
            if data['status'] == 'success' and data['data']['result']:
                # Return list of float values from the first result series
                values = data['data']['result'][0]['values']
                # values is a list of [timestamp, "value_string"]
                return [float(v[1]) for v in values if v[1] != "NaN"]
        except (json.JSONDecodeError, KeyError, IndexError):
            pass
    return []

def get_metadata(folder_path):
    """Extract mode and replicas from folder path for both single and batch experiments"""
    basename = os.path.basename(folder_path)
    parent_basename = os.path.basename(os.path.dirname(folder_path))
    
    # Batch experiment format: [mode]_[timestamp] / replicas_[N]
    if basename.startswith('replicas_'):
        replicas = basename.replace('replicas_', '')
        mode = 'iptables' if 'iptables' in parent_basename else 'nftables'
    # Single experiment format: [mode]_[N]pods_[timestamp]
    else:
        mode = 'iptables' if 'iptables' in basename else 'nftables'
        parts = basename.split('_')
        replicas = parts[1].replace('pods', '') if len(parts) > 1 else "Unknown"
        
    return mode, int(replicas) if replicas.isdigit() else 0

def plot_metric_comparison(folders, metric_filename, title, ylabel, output_file, is_timeseries=True):
    """Plot timeseries data comparing iptables vs nftables for a specific metric"""
    plt.figure(figsize=(12, 7))
    
    plotted_any = False
    
    # Generate unique colors for each replica count and linestyles for modes
    all_replicas = sorted(list(set([get_metadata(f)[1] for f in folders])))
    cmap = plt.get_cmap('tab10')
    replica_colors = {r: cmap(i % 10) for i, r in enumerate(all_replicas)}
    line_styles = {'iptables': '--', 'nftables': '-'}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        filepath = os.path.join(folder, metric_filename)
        
        data = load_timeseries_data(filepath)
        
        if data:
            # Use a stable 1s step size over raw json data dumps
            step_size = 1
            
            # Time in seconds
            time_x = [i * step_size for i in range(len(data))]
            label = f"{mode.capitalize()} ({replicas})"
            plt.plot(time_x, data, label=label, color=replica_colors[replicas], linestyle=line_styles.get(mode, '-'), linewidth=2, alpha=0.8)
            plotted_any = True
    
    if not plotted_any:
        print(f"Skipping {title} - No valid timeseries data found.")
        plt.close()
        return

    plt.title(title, fontsize=14, pad=15)
    plt.xlabel('Time (seconds since start)', fontsize=12)
    plt.ylabel(ylabel, fontsize=12)
    plt.legend(loc='best')
    plt.grid(True, linestyle='--', alpha=0.7)
    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")

def plot_bar_comparison(folders, metric_filename, title, ylabel, output_file, is_memory=False, is_average=False, is_integral=False):
    """Plot a grouped bar chart comparing the metric value across different pod counts."""
    data_map = {}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        filepath = os.path.join(folder, metric_filename)
        data = load_timeseries_data(filepath)
        
        if data:
            # Use a stable 1s step size over raw json data dumps
            step_size = 1
            
            if is_integral:
                # Calculate absolute total energy expended (Area under the curve)
                # Multiply each rate by the step_size.
                metric_value = sum([val * step_size for val in data])
            elif is_average:
                # Calculate the overall average across the entire timeseries (Flawed for CPU, kept for Memory)
                metric_value = sum(data) / len(data)
            else:
                # Take the average of the last 3 points to represent final stable state avoiding 1-sec spikes
                metric_value = sum(data[-3:]) / len(data[-3:]) if len(data) >= 3 else data[-1]
                
            if is_memory:
                metric_value = metric_value / (1024 * 1024) # Convert bytes to MB
                
            if replicas not in data_map:
                data_map[replicas] = {}
            data_map[replicas][mode] = metric_value
            
    if not data_map:
        return

    sorted_replicas = sorted(list(data_map.keys()))
    iptables_vals = [data_map[r].get('iptables', 0) for r in sorted_replicas]
    nftables_vals = [data_map[r].get('nftables', 0) for r in sorted_replicas]

    x = range(len(sorted_replicas))
    width = 0.35

    fig, ax = plt.subplots(figsize=(10, 6))
    ax.bar([i - width/2 for i in x], iptables_vals, width, label='Iptables', color=COLORS.get('iptables'), alpha=0.9)
    ax.bar([i + width/2 for i in x], nftables_vals, width, label='Nftables', color=COLORS.get('nftables'), alpha=0.9)

    ax.set_xlabel('Number of Pods', fontsize=12)
    ax.set_ylabel(ylabel, fontsize=12)
    ax.set_title(title, fontsize=14, pad=15)
    ax.set_xticks(list(x))
    ax.set_xticklabels(sorted_replicas)
    ax.legend()
    ax.grid(True, axis='y', linestyle='--', alpha=0.7)
    
    # Add value labels on top of bars
    for i, v in enumerate(iptables_vals):
        if v > 0:
            ax.text(i - width/2, v + (max(max(iptables_vals), max(nftables_vals)) * 0.01), f'{v:.2f}', ha='center', va='bottom', fontsize=9, rotation=90)
    for i, v in enumerate(nftables_vals):
        if v > 0:
            ax.text(i + width/2, v + (max(max(iptables_vals), max(nftables_vals)) * 0.01), f'{v:.2f}', ha='center', va='bottom', fontsize=9, rotation=90)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")


def plot_total_duration_comparison(folders, output_file):
    """Plot a grouped bar chart comparing the total scale-up time (excluding 30s buffer)."""
    data_map = {}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        start_file = os.path.join(folder, 'deploy_start_timestamp.txt')
        end_file = os.path.join(folder, 'deploy_end_timestamp.txt')
        
        if os.path.exists(start_file) and os.path.exists(end_file):
            try:
                with open(start_file, 'r') as sf, open(end_file, 'r') as ef:
                    start_time = int(sf.read().strip())
                    end_time = int(ef.read().strip())
                    
                    # Subtract 30 seconds to exclude the trailing metric collection buffer
                    duration = max(0, end_time - start_time - 30)
                    
                    if replicas not in data_map:
                        data_map[replicas] = {}
                    data_map[replicas][mode] = duration
            except (ValueError, IOError):
                pass
            
    if not data_map:
        return

    sorted_replicas = sorted(list(data_map.keys()))
    iptables_vals = [data_map[r].get('iptables', 0) for r in sorted_replicas]
    nftables_vals = [data_map[r].get('nftables', 0) for r in sorted_replicas]

    x = range(len(sorted_replicas))
    width = 0.35

    fig, ax = plt.subplots(figsize=(10, 6))
    ax.bar([i - width/2 for i in x], iptables_vals, width, label='Iptables', color=COLORS.get('iptables'), alpha=0.9)
    ax.bar([i + width/2 for i in x], nftables_vals, width, label='Nftables', color=COLORS.get('nftables'), alpha=0.9)

    ax.set_xlabel('Number of Pods', fontsize=12)
    ax.set_ylabel('Total Scale-up Duration (Seconds)', fontsize=12)
    ax.set_title('Total Experiment Run Time (Excluding 30s Stabilization Buffer)', fontsize=14, pad=15)
    ax.set_xticks(list(x))
    ax.set_xticklabels(sorted_replicas)
    ax.legend()
    ax.grid(True, axis='y', linestyle='--', alpha=0.7)
    
    # Add value labels on top of bars
    for i, v in enumerate(iptables_vals):
        if v > 0:
            ax.text(i - width/2, v + (max(max(iptables_vals), max(nftables_vals)) * 0.01), f'{v}s', ha='center', va='bottom', fontsize=9, rotation=90)
    for i, v in enumerate(nftables_vals):
        if v > 0:
            ax.text(i + width/2, v + (max(max(iptables_vals), max(nftables_vals)) * 0.01), f'{v}s', ha='center', va='bottom', fontsize=9, rotation=90)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")


def plot_duration_line_trend(folders, output_file):
    """Plot a line graph showing scaling trend of total duration vs pod count."""
    data_map = {}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        start_file = os.path.join(folder, 'deploy_start_timestamp.txt')
        end_file = os.path.join(folder, 'deploy_end_timestamp.txt')
        
        if os.path.exists(start_file) and os.path.exists(end_file):
            try:
                with open(start_file, 'r') as sf, open(end_file, 'r') as ef:
                    start_time = int(sf.read().strip())
                    end_time = int(ef.read().strip())
                    duration = max(0, end_time - start_time - 30)
                    
                    if replicas not in data_map:
                        data_map[replicas] = {}
                    data_map[replicas][mode] = duration
            except (ValueError, IOError):
                pass
            
    if not data_map:
        return

    sorted_replicas = sorted(list(data_map.keys()))
    iptables_vals = [data_map[r].get('iptables', None) for r in sorted_replicas]
    nftables_vals = [data_map[r].get('nftables', None) for r in sorted_replicas]

    plt.figure(figsize=(10, 6))
    
    ip_x = [r for r, v in zip(sorted_replicas, iptables_vals) if v is not None]
    ip_y = [v for v in iptables_vals if v is not None]
    nf_x = [r for r, v in zip(sorted_replicas, nftables_vals) if v is not None]
    nf_y = [v for v in nftables_vals if v is not None]

    if ip_x:
        plt.plot(ip_x, ip_y, label='Iptables', color=COLORS.get('iptables'), marker='o', linestyle='--', linewidth=2, markersize=8)
    if nf_x:
        plt.plot(nf_x, nf_y, label='Nftables', color=COLORS.get('nftables'), marker='s', linestyle='-', linewidth=2, markersize=8)

    plt.xlabel('Number of Pods', fontsize=12)
    plt.ylabel('Total Scale-up Duration (Seconds)', fontsize=12)
    plt.title('Scale-up Time vs Pod Count (Trend)', fontsize=14, pad=15)
    plt.grid(True, linestyle='--', alpha=0.7)
    plt.legend()
    plt.xticks(sorted_replicas)
    
    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")


def plot_duration_line_trend(folders, output_file):
    """Plot a line graph showing scaling trend of total duration vs pod count."""
    data_map = {}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        start_file = os.path.join(folder, 'deploy_start_timestamp.txt')
        end_file = os.path.join(folder, 'deploy_end_timestamp.txt')
        
        if os.path.exists(start_file) and os.path.exists(end_file):
            try:
                with open(start_file, 'r') as sf, open(end_file, 'r') as ef:
                    start_time = int(sf.read().strip())
                    end_time = int(ef.read().strip())
                    duration = max(0, end_time - start_time - 30)
                    
                    if replicas not in data_map:
                        data_map[replicas] = {}
                    data_map[replicas][mode] = duration
            except (ValueError, IOError):
                pass
            
    if not data_map:
        return

    sorted_replicas = sorted(list(data_map.keys()))
    iptables_vals = [data_map[r].get('iptables', None) for r in sorted_replicas]
    nftables_vals = [data_map[r].get('nftables', None) for r in sorted_replicas]

    plt.figure(figsize=(10, 6))
    
    ip_x = [r for r, v in zip(sorted_replicas, iptables_vals) if v is not None]
    ip_y = [v for v in iptables_vals if v is not None]
    nf_x = [r for r, v in zip(sorted_replicas, nftables_vals) if v is not None]
    nf_y = [v for v in nftables_vals if v is not None]

    if ip_x:
        plt.plot(ip_x, ip_y, label='Iptables', color=COLORS.get('iptables'), marker='o', linestyle='--', linewidth=2, markersize=8)
    if nf_x:
        plt.plot(nf_x, nf_y, label='Nftables', color=COLORS.get('nftables'), marker='s', linestyle='-', linewidth=2, markersize=8)

    plt.xlabel('Number of Pods', fontsize=12)
    plt.ylabel('Total Scale-up Duration (Seconds)', fontsize=12)
    plt.title('Scale-up Time vs Pod Count (Trend)', fontsize=14, pad=15)
    plt.grid(True, linestyle='--', alpha=0.7)
    plt.legend()
    plt.xticks(sorted_replicas)
    
    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")


def plot_throughput_efficiency(folders, output_file):
    """Plot CPU Cost per Sync (CPU Cores / Syncs per second) acting as an efficiency metric."""
    data_map = {}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        cpu_file = os.path.join(folder, 'cpu_usage_timeseries.json')
        sync_count_file = os.path.join(folder, 'sync_count_timeseries.json')
        
        cpu_data = load_timeseries_data(cpu_file)
        sync_data = load_timeseries_data(sync_count_file)
        
        # Only process if both metrics exist and have data
        if cpu_data and sync_data:
            # Match lengths in case arrays slightly differ
            min_len = min(len(cpu_data), len(sync_data))
            
            efficiencies = []
            for i in range(min_len):
                cpu = cpu_data[i]
                syncs = sync_data[i]
                # Avoid division by zero when proxy is completely idle
                if syncs > 0:
                    efficiencies.append(cpu / syncs)
                    
            if efficiencies:
                # Average the efficiency over the active period
                avg_efficiency = sum(efficiencies) / len(efficiencies)
                
                if replicas not in data_map:
                    data_map[replicas] = {}
                data_map[replicas][mode] = avg_efficiency
                
    if not data_map:
        print("Skipping Throughput Efficiency - missing CPU or Sync Count json files.")
        return

    sorted_replicas = sorted(list(data_map.keys()))
    iptables_vals = [data_map[r].get('iptables', 0) for r in sorted_replicas]
    nftables_vals = [data_map[r].get('nftables', 0) for r in sorted_replicas]

    x = range(len(sorted_replicas))
    width = 0.35

    fig, ax = plt.subplots(figsize=(10, 6))
    ax.bar([i - width/2 for i in x], iptables_vals, width, label='Iptables', color=COLORS.get('iptables'), alpha=0.9)
    ax.bar([i + width/2 for i in x], nftables_vals, width, label='Nftables', color=COLORS.get('nftables'), alpha=0.9)

    ax.set_xlabel('Number of Pods', fontsize=12)
    ax.set_ylabel('CPU Cores per 1 Sync/sec', fontsize=12)
    ax.set_title('CPU Cost per Sync (Throughput Efficiency)', fontsize=14, pad=15)
    ax.set_xticks(list(x))
    ax.set_xticklabels(sorted_replicas)
    ax.legend()
    ax.grid(True, axis='y', linestyle='--', alpha=0.7)

    plt.tight_layout()
    plt.savefig(output_file, dpi=300)
    plt.close()
    print(f"Generated {output_file}")


def main():
    parser = argparse.ArgumentParser(description='Generate plots for Kube-Proxy experiment results')
    parser.add_argument('--results-dir', required=True, help='Directory containing the result folders (e.g., iptables_15000pods_...)')
    parser.add_argument('--output-dir', default='.', help='Directory to save the generated plots')
    args = parser.parse_args()

    # Find all result directories
    folders = []
    # 1. Support single experiment format: [mode]_[replicas]pods_[timestamp]
    folders.extend(glob.glob(os.path.join(args.results_dir, '*_*pods_*')))
    # 2. Support batch experiment format: [mode]_[timestamp]/replicas_[replicas]
    folders.extend(glob.glob(os.path.join(args.results_dir, '*_*', 'replicas_*')))
    
    if not folders:
        print(f"No result directories found in {args.results_dir}")
        print("Expected folder format: [mode]_[replicas]pods_[timestamp] OR [mode]_[timestamp]/replicas_[replicas]")
        return

    # Sort folders by number of replicas for consistent plotting
    folders.sort(key=lambda x: get_metadata(x)[1])

    print(f"Found {len(folders)} experiment directories.")
    os.makedirs(args.output_dir, exist_ok=True)

    # 1. Sync Duration (The core metric)
    for p_level in ['p99', 'p95', 'p50']:
        plot_metric_comparison(
            folders, f'sync_duration_{p_level}_timeseries.json',
            f'Kube-Proxy Rule Sync Duration Per Cycle ({p_level.replace("p","")}th Percentile)', 
            'Duration (Seconds)', 
            os.path.join(args.output_dir, f'plot_sync_duration_{p_level}.png')
        )
        plot_bar_comparison(
            folders, f'sync_duration_{p_level}_timeseries.json',
            f'Final Sync Duration by Pod Count ({p_level.replace("p","")}th Percentile)', 
            'Duration (Seconds)', 
            os.path.join(args.output_dir, f'bar_sync_duration_{p_level}.png')
        )

    # 2. CPU Usage (kube-proxy only)
    plot_metric_comparison(
        folders, 'cpu_usage_timeseries.json',
        'Kube-Proxy Only - CPU Consumption Rate', 
        'CPU Cores Consumed by Kube-Proxy', 
        os.path.join(args.output_dir, 'plot_cpu_usage.png')
    )
    plot_bar_comparison(
        folders, 'cpu_usage_timeseries.json',
        'Kube-Proxy Only - Total CPU Seconds Consumed', 
        'Total CPU-Seconds Expended by Kube-Proxy', 
        os.path.join(args.output_dir, 'bar_cpu_usage.png'),
        is_integral=True
    )
    
    # 2.5 Overall System CPU Usage (Node Level)
    plot_metric_comparison(
        folders, 'overall_cpu_usage_timeseries.json',
        'System Wide - Overall Node CPU Utilization', 
        'Total Node CPU Utilization (%)', 
        os.path.join(args.output_dir, 'plot_overall_cpu_usage.png')
    )
    plot_bar_comparison(
        folders, 'overall_cpu_usage_timeseries.json',
        'System Wide - Average Node CPU Utilization by Pod Count', 
        'Average Total Node CPU Utilization (%)', 
        os.path.join(args.output_dir, 'bar_overall_cpu_usage.png'),
        is_average=True
    )

    # 3. Memory Usage
    # For memory, we convert Bytes to Megabytes
    plt.figure(figsize=(12, 7))
    plotted_mem = False
    
    all_replicas = sorted(list(set([get_metadata(f)[1] for f in folders])))
    cmap = plt.get_cmap('tab10')
    replica_colors = {r: cmap(i % 10) for i, r in enumerate(all_replicas)}
    line_styles = {'iptables': '--', 'nftables': '-'}
    
    for folder in folders:
        mode, replicas = get_metadata(folder)
        filepath = os.path.join(folder, 'memory_usage_timeseries.json')
        data = load_timeseries_data(filepath)
        if data:
            # Use a stable 1s step size over raw json data dumps
            step_size = 1
            
            # Convert bytes to MB
            data_mb = [v / (1024 * 1024) for v in data]
            time_x = [i * step_size for i in range(len(data_mb))]
            label = f"{mode.capitalize()} ({replicas})"
            plt.plot(time_x, data_mb, label=label, color=replica_colors[replicas], linestyle=line_styles.get(mode, '-'), linewidth=2)
            plotted_mem = True
            
    if plotted_mem:
        plt.title('Kube-Proxy Memory Consumption', fontsize=14, pad=15)
        plt.xlabel('Time (seconds since start)', fontsize=12)
        plt.ylabel('Memory (MB)', fontsize=12)
        plt.legend(loc='best')
        plt.grid(True, linestyle='--', alpha=0.7)
        plt.tight_layout()
        plt.savefig(os.path.join(args.output_dir, 'plot_memory_usage.png'), dpi=300)
    plt.close()

    if plotted_mem:
        print(f"Generated {os.path.join(args.output_dir, 'plot_memory_usage.png')}")
        
    plot_bar_comparison(
        folders, 'memory_usage_timeseries.json',
        'Final Stable Memory Consumption by Pod Count', 
        'Memory (MB)', 
        os.path.join(args.output_dir, 'bar_memory_usage.png'),
        is_memory=True
    )

    # 4. Network Programming Latency
    for p_level in ['p99', 'p95', 'p50']:
        plot_metric_comparison(
            folders, f'network_programming_{p_level}_timeseries.json',
            f'End-to-End Network Programming Latency ({p_level.replace("p","")}th Percentile)', 
            'Latency (Seconds)', 
            os.path.join(args.output_dir, f'plot_network_programming_{p_level}.png')
        )
        plot_bar_comparison(
            folders, f'network_programming_{p_level}_timeseries.json',
            f'Final Network Latency by Pod Count ({p_level.replace("p","")}th Percentile)', 
            'Latency (Seconds)', 
            os.path.join(args.output_dir, f'bar_network_programming_{p_level}.png')
        )
        
    # 4.5 KWOK Pod Spawning Latency
    for p_level in ['p99', 'p95', 'p50']:
        plot_metric_comparison(
            folders, f'kwok_pod_duration_{p_level}_timeseries.json',
            f'KWOK Pod Spawning Duration ({p_level.replace("p","")}th Percentile)', 
            'Duration (Seconds)', 
            os.path.join(args.output_dir, f'plot_kwok_pod_duration_{p_level}.png')
        )
        plot_bar_comparison(
            folders, f'kwok_pod_duration_{p_level}_timeseries.json',
            f'Final KWOK Pod Spawning Duration by Pod Count ({p_level.replace("p","")}th Percentile)', 
            'Duration (Seconds)', 
            os.path.join(args.output_dir, f'bar_kwok_pod_duration_{p_level}.png')
        )

    # 5. Total Experiment Duration
    plot_total_duration_comparison(
        folders, 
        os.path.join(args.output_dir, 'bar_total_experiment_duration.png')
    )
    
    # 6. Trend line for Experiment Duration
    plot_duration_line_trend(
        folders, 
        os.path.join(args.output_dir, 'line_experiment_duration_trend.png')
    )

    # 7. CPU Cost per Sync (Throughput Efficiency)
    plot_throughput_efficiency(
        folders,
        os.path.join(args.output_dir, 'bar_throughput_efficiency.png')
    )

if __name__ == "__main__":
    main()