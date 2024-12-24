from scipy import stats
import pandas as pd
import json
import argparse
import numpy as np
from mapper import load_trace, generate_plot
import matplotlib.pyplot as plt
import os
from log_config import *

def generate_distributions(trace_directorypath, profile_filepath, output_filepath):
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
        log.critical(f"Load Generation failed")
        return
    
    trace_durations = {}
    mapped_durations = {}
    for trace in trace_functions:
        duration = trace_functions[trace]["duration"]["50-percentile"]
        proxy_name = mapped_traces[trace]["proxy-function"]
        profile_duration = proxy_functions[proxy_name]["compute_duration"]["50-percentile"]
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
        help="Sampled function traces"
    )
    args = parser.parse_args()
    trace_directory = args.trace_directory
    sample_directory = args.sampled_directory
    profile_directory = args.profile_directory

    trace_functions, err = load_trace(trace_directory)
    trace_durations = []
    for function in trace_functions:
        trace_durations.append(trace_functions[function]["duration"]["50-percentile"])
    samples = os.listdir(sample_directory)
    sample_sizes = []
    wd_distances_sample, wd_distances_mapped = {}, {}
    for sample in samples:
        sample_durations, mapped_durations = generate_plot(os.path.join(sample_directory, sample), profile_directory, os.path.join(sample_directory, sample)+"/mapper_output.json")
        sample_size = len(sample_durations)
        sample_distance = stats.wasserstein_distance(trace_durations, sample_durations)
        mapped_distance = stats.wasserstein_distance(trace_durations, mapped_durations)
        if sample_size in wd_distances_sample:
            wd_distances_sample[sample_size].append(sample_distance)
        else:
            wd_distances_sample[sample_size] = [sample_distance]
        if sample_size in wd_distances_mapped:
            wd_distances_mapped[sample_size].append(mapped_distance)
        else:
            wd_distances_mapped[sample_size] = [mapped_distance]
        sample_sizes.append(sample_size)
    
    wd_samples, wd_mapped = [], []
    for size in sample_sizes:
        wd_samples.append(wd_distances_sample[size][0])
        wd_distances_sample[size].pop(0)
        wd_mapped.append(wd_distances_mapped[size][0])
        wd_distances_mapped[size].pop(0)

    #fig, ax1 = plt.subplots()
    # Plot the first line on the left y-axis
    plt.ylabel('Wasserstein Distance')
    plt.xlabel('Sample Size')
    plt.plot(sample_sizes, wd_mapped, label='Mapped WD')
    plt.plot(sample_sizes, wd_samples, label="Sampled WD")
    
    plt.legend()
    # Show the plot
    plt.savefig('WD_distributions.png')


if __name__ == "__main__":
    main()