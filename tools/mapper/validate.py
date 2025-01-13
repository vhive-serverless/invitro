from scipy import stats
import pandas as pd
import json
import argparse
import numpy as np
from mapper import load_trace, generate_plot, INVOCATION_COLUMN
import matplotlib.pyplot as plt
import os
from log_config import *

def cdf_invocations(trace_dir, profile_filepath):
    invocations = pd.read_csv(os.path.join(trace_dir, "invocations.csv"))
    trace_durations, mapped_durations = extract_durations(trace_dir, profile_filepath, trace_dir+"/mapper_output.json")
    num_invocations = {}
    for i in range(len(invocations)):
        if invocations["HashFunction"][i] not in trace_durations:
            continue
        num_invocations[invocations["HashFunction"][i]] = sum([invocations[col][i] if col not in invocations.columns[0:INVOCATION_COLUMN] else 0 for col in invocations.columns])

    return [trace_durations[trace]*num_invocations[trace] for trace in num_invocations], [mapped_durations[trace]*num_invocations[trace] for trace in mapped_durations], len(invocations)

def group_by_invocations(trace_dir, profile_filepath):
    sample_inv_df, mapped_inv_df = {}, {}
    samples = os.listdir(trace_dir)
    for sample in samples:
        invocations = pd.read_csv(os.path.join(trace_dir, sample)+"/invocations.csv")
        sample_durations, mapped_durations = extract_durations(os.path.join(trace_dir, sample), profile_filepath, os.path.join(trace_dir, sample)+"/mapper_output.json")
        for i in range(len(invocations)):
            num_invocations = sum([invocations[col][i] if col not in invocations.columns[0:INVOCATION_COLUMN] else 0 for col in invocations.columns])
            trace_hash = invocations["HashFunction"][i]
            #fetch sample_durations
            if num_invocations not in sample_inv_df:
                sample_inv_df[num_invocations] = [sample_durations[trace_hash]]
                mapped_inv_df[num_invocations] = [mapped_durations[trace_hash]]
            else:
                sample_inv_df[num_invocations].append(sample_durations[trace_hash])
                mapped_inv_df[num_invocations].append(mapped_durations[trace_hash])
    return sample_inv_df, mapped_inv_df

def extract_durations(trace_directorypath, profile_filepath, output_filepath):
    trace_functions, err = load_trace(trace_directorypath)
    if err == -1:
        log.critical(f"Load Generation failed")
        return
    elif err == 0:
        log.info(f"Trace loaded")

    ## Check whether the profile file for proxy functions exists or not
    if os.path.exists(profile_filepath):
        log.info(
            f"Profile file for proxy functions {profile_filepath} exists. Accessing information"
        )
        try:
            with open(profile_filepath, "r") as jf:
                proxy_functions = json.load(jf)
        except Exception as e:
            log.critical(
                f"Profile file for proxy functions {profile_filepath} cannot be read. Error: {e}"
            )
            log.critical(f"Load Generation failed")
            return
    else:
        log.critical(f"Profile file for proxy functions {profile_filepath} not found")
        log.critical(f"Load Generation failed")
        return
    
    if os.path.exists(output_filepath):
        log.info(
            f"Mapper output file for trace functions {output_filepath} exists. Accessing information"
        )
        try:
            with open(output_filepath, "r") as jf:
                mapped_traces = json.load(jf)
        except Exception as e:
            log.critical(
                f"Mapper output file for trace functions {output_filepath} cannot be read. Error: {e}"
            )
            log.critical(f"Load Generation failed")
            return
    else:
        log.critical(f"Mapper output file for trace functions {output_filepath} not found")
        trace_durations = {}
        mapped_durations = {}
        for trace in trace_functions:
            duration = trace_functions[trace]["duration"]["50-percentile"]
            if duration > 2000:
                continue
            trace_durations[trace] = duration
        return trace_durations, []
    
    trace_durations = {}
    mapped_durations = {}
    for trace in trace_functions:
        duration = trace_functions[trace]["duration"]["50-percentile"]
        if duration > 2000:
            continue
        proxy_name = mapped_traces[trace]["proxy-function"]
        profile_duration = proxy_functions[proxy_name]["duration"]["50-percentile"]
        trace_durations[trace] = duration
        mapped_durations[trace] = profile_duration
    return trace_durations, mapped_durations

def main():
    parser = argparse.ArgumentParser(description="Validator")
    parser.add_argument(
        "-t",
        "--trace-directory",
        help="Original traces",
    )
    parser.add_argument(
        "-p",
        "--profile-directory",
        help="Profile directory",
    )
    parser.add_argument(
        "-s",
        "--sampled-directory",
        help="Sampled function traces",
    )
    parser.add_argument(
        "-i",
        "--invocations",
        help="Group by invocation instead of sample",
    )
    parser.add_argument(
        "-c",
        "--cdf-invocations",
        help="Plot CDF of duration of invocations",
        default=False,
    )
    parser.add_argument(
        "-o",
        "--output",
        help="Output file name",
        default="WD.png",
    )
    args = parser.parse_args()
    trace_directory = args.trace_directory
    sample_directory = args.sampled_directory
    profile_directory = args.profile_directory
    inv_f = args.invocations
    output_filename = args.output
    cdf_invoked = args.cdf_invocations
    trace_functions, err = load_trace(trace_directory)
    trace_durations = []
    if cdf_invoked:
        trace_durations, _, _ = cdf_invocations(trace_directory, profile_directory)
    else:
        for function in trace_functions:
            if trace_functions[function]["duration"]["50-percentile"] > 2000:
                continue
            trace_durations.append(trace_functions[function]["duration"]["50-percentile"])
    
    samples = os.listdir(sample_directory)
    if inv_f:
        log.info("Grouping by Invocations")
        s_inv_df, m_inv_df = group_by_invocations(sample_directory, profile_directory)
        wd_distances_sample, wd_distances_mapped, pct_delta = [], [], []
        invocations = sorted(s_inv_df.keys())
        for invocation in invocations:
            sample_distance = stats.wasserstein_distance(trace_durations, s_inv_df[invocation])
            mapped_distance = stats.wasserstein_distance(trace_durations, m_inv_df[invocation])
            wd_distances_sample.append(sample_distance)
            wd_distances_mapped.append(mapped_distance)
            pct_delta_sample = (sum([1 if abs(s_inv_df[invocation][i]-m_inv_df[invocation][i])/m_inv_df[invocation][i] <= 0.2 else 0 for i in range(len(s_inv_df[invocation]))])/len(s_inv_df[invocation]))*100
            pct_delta.append(pct_delta_sample)
        
        print(wd_distances_mapped)
        plt.ylabel('Wasserstein Distance')
        plt.xlabel('No. of Invocations')
        plt.plot(invocations, wd_distances_sample, label="Sampled WD")
        plt.plot(invocations, wd_distances_mapped, label='Mapped WD')
        plt.legend()
        # Show the plot
        plt.savefig(output_filename+".png")
        fig = plt.figure()
        ax = fig.add_subplots(111)

        ax.plot(invocations, pct_delta)
        ax.set_ylabel('Pct traces falling in 10 pct duration delta')
        ax.set_xlabel('No. of invocations')
        plt.savefig(output_filename+"_pct_diff.png")
        return       
    sample_sizes = []
    wd_distances_sample, wd_distances_mapped, dropped_functions_sample, dropped_invocations_sample = {}, {}, {}, {}
    for sample in samples:
        if cdf_invoked:
            sample_durations, mapped_durations, sample_size = cdf_invocations(os.path.join(sample_directory, sample), profile_directory)
        else:
            sample_durations, mapped_durations, dropped_functions, dropped_invocations = generate_plot(os.path.join(sample_directory, sample), profile_directory, os.path.join(sample_directory, sample)+"/mapper_output.json")
            sample_size = len(sample_durations)+dropped_functions
            dropped_functions_sample[sample_size] = dropped_functions
            dropped_invocations_sample[sample_size] = dropped_invocations
        sample_distance = stats.wasserstein_distance(trace_durations, sample_durations)
        mapped_distance = stats.wasserstein_distance(trace_durations, mapped_durations)
        wd_distances_sample[sample_size] = sample_distance
        wd_distances_mapped[sample_size] = mapped_distance
        sample_sizes.append(sample_size)
    
    sample_sizes = sorted(sample_sizes)
    wd_mapped, wd_samples = [wd_distances_mapped[size] for size in sample_sizes], [wd_distances_sample[size] for size in sample_sizes]
    if not cdf_invoked:
        functions, invocations = [dropped_functions_sample[size] for size in sample_sizes], [dropped_invocations_sample[size] for size in sample_sizes] 
    #fig, ax1 = plt.subplots()
    # Plot the first line on the left y-axis
    if not cdf_invoked:
        plt.ylabel('Wasserstein Distance of Duration CDF')
    else:
        plt.ylabel('Wasserstein Distance of Duration of Invocation CDF')
    plt.xlabel('Sample Size')
    ln1, = plt.plot(sample_sizes, wd_mapped, label='Mapped WD')
    ln2, = plt.plot(sample_sizes, wd_samples, label="Sampled WD")
    
    plt.legend()
    # Show the plot
    plt.savefig(output_filename+".png")
    if not cdf_invoked:
        fig, axs = plt.subplots(2, 1)
        axs[0].plot(sample_sizes, [(functions[i] / sample_sizes[i])*100 for i in range(len(functions))])
        axs[0].set_ylabel('Pct dropped functions')
        axs[0].set_xlabel('Sample Size')
        axs[1].plot(sample_sizes, invocations)
        axs[1].set_xlabel('Sample Size')
        axs[1].set_ylabel('Pct dropped invocations')
        plt.tight_layout()
        plt.savefig(output_filename+"_dropped_data.png")


if __name__ == "__main__":
    main()