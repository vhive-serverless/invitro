import os

import json
import re
import argparse
import pandas as pd
import matplotlib.pyplot as plt
from find_proxy_function import *

from log_config import *

INVOCATION_COLUMN = 4

def load_trace(trace_directorypath):
    duration_info = {}
    memory_info = {}
    # Read the trace files and store the information

    # Read the durations file
    duration_filepath = trace_directorypath + "/durations.csv"
    if os.path.exists(duration_filepath):
        log.info(f"Durations file {duration_filepath} exists. Accessing information")
        try:
            duration_info = pd.read_csv(duration_filepath)
        except Exception as e:
            log.critical(
                f"Durations file {duration_filepath} cannot be read. Error: {e}"
            )
            return None, -1

    # Read the memory file
    memory_filepath = trace_directorypath + "/memory.csv"
    if os.path.exists(memory_filepath):
        log.info(f"Memory file {memory_filepath} exists. Accessing information")
        try:
            memory_info = pd.read_csv(memory_filepath)
        except Exception as e:
            log.critical(
                f"Memory file {memory_filepath} cannot be read. Error: {e}"
            )
            return None, -1
        
    # Rename all columns in the dataframe with a lambda (for example: percentile_Average_1 -> 1-percentile) if x matches a regex
    duration_info = duration_info.rename(columns=lambda x: re.sub(r'percentile_(\w+)_(\d+)', r'\2-' + "percentile", x))
    # Rename all columns in the dataframe with a lambda (for example: AverageAllocatedMb_pct1 -> 1-percentile) if x matches a regex
    memory_info = memory_info.rename(columns=lambda x: re.sub(r'AverageAllocatedMb_pct(\w+)', r'\1-' + "percentile", x))

    # Add them to a dict with the key being the HashFunction
    trace_functions = {}

    for _, row in duration_info.iterrows():
        hash_function = row["HashFunction"]
        trace_functions[hash_function] = {}
        trace_functions[hash_function]["duration"] = row

    for _, row in memory_info.iterrows():
        hash_function = row["HashFunction"]
        trace_functions[hash_function]["memory"] = row

    return trace_functions, 0


def generate_trace(trace_directorypath, profile_filepath, output_filepath, unique_assignment, mode):
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

    # Getting a proxy function for every trace function
    trace_functions, err = get_proxy_function(
        trace_functions=trace_functions,
        proxy_functions=proxy_functions,
        unique_assignment=unique_assignment,
        mode=mode,
    )
    if err == -1:
        log.critical(f"Load Generation failed")
        return
    elif err == 0:
        log.info(f"Proxy functions obtained")

    # Writing the proxy functions to a file

    # Only give function name and proxy name
    trace_json = {}
    for function in trace_functions:
        trace_json[function] = {}
        trace_json[function]["proxy-function"] = trace_functions[function]["proxy-function"] 

    try:
        with open(output_filepath, "w") as jf:
            json.dump(trace_json, jf, indent=4)
    except Exception as e:
        log.critical(f"Output file {output_filepath} cannot be written. Error: {e}")
        log.critical(f"Load Generation failed")
        return
    
    log.info(f"Output file {output_filepath} written")
    log.info(f"Load Generation successful")

def generate_plot(trace_directorypath, profile_filepath, output_filepath, invoke=True):
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
    
    if invoke:
        invocations = pd.read_csv(trace_directorypath+"/invocations.csv")
        inv_df = {}
        for i in range(len(invocations)):
            inv_df[invocations["HashFunction"][i]] = sum([invocations[col][i] if col not in invocations.columns[0:INVOCATION_COLUMN] else 0 for col in invocations.columns])
        
        trace_durations = []
        mapped_durations = []
        dropped_functions, dropped_invocations, total_invocations = 0, 0, 0
        for trace in trace_functions:
            duration = trace_functions[trace]["duration"]["50-percentile"]
            if duration > 2000:
                dropped_functions += 1
                dropped_invocations += inv_df[trace]
                continue
            total_invocations += inv_df[trace]
            proxy_name = mapped_traces[trace]["proxy-function"]
            profile_duration = proxy_functions[proxy_name]["duration"]["50-percentile"]
            trace_durations.append(duration)
            mapped_durations.append(profile_duration)
        return trace_durations, mapped_durations, dropped_functions, (dropped_invocations/total_invocations)*100
    else:
        trace_durations = []
        mapped_durations = []
        for trace in trace_functions:
            duration = trace_functions[trace]["duration"]["50-percentile"]
            if duration > 2000:
                continue
            proxy_name = mapped_traces[trace]["proxy-function"]
            profile_duration = proxy_functions[proxy_name]["duration"]["50-percentile"]
            trace_durations.append(duration)
            mapped_durations.append(profile_duration)
        return trace_durations, mapped_durations        


def main():
    # Parse the arguments
    parser = argparse.ArgumentParser(description="Mapper")
    parser.add_argument(
        "-t",
        "--trace-directorypath",
        type=str,
        help="Path to the directory containing the trace files",
        required=True,
    )
    parser.add_argument(
        "-p",
        "--profile-filepath",
        type=str,
        help="Path to the profile file containing the proxy functions",
        required=True,
    )
    parser.add_argument(
        "-u",
        "--unique-assignment",
        type=bool,
        help="Whether to assign unique proxy functions to each trace function",
        default=False,
        required=False,
    )
    parser.add_argument(
        "-m",
        "--mode",
        type=str,
        help="Whether to use EcoFaaS functions or not",
        default="vSwarm",
        required=False,
    )
    parser.add_argument(
        "-q",
        "--multi-mode",
        type=bool,
        help="Whether to parse multiple traces",
        default=False,
        required=False,
    )
    parser.add_argument(
        "-l",
        "--plot",
        type=bool,
        help="Plot mapped traces",
        default=False,
        required=False,
    )
    args = parser.parse_args()
    trace_directorypath = args.trace_directorypath
    profile_filepath = args.profile_filepath
    output_filepath = trace_directorypath + "/mapper_output.json"
    unique_assignment = args.unique_assignment
    mode = args.mode
    multi_mode = args.multi_mode
    plot = args.plot
    if multi_mode or plot:
        trace_durations = []
        mapped_durations = []
        profile_dirs = os.listdir(trace_directorypath)
        for profile in profile_dirs:
            tracepath = os.path.join(trace_directorypath, profile)
            outputpath = os.path.join(tracepath, "mapper_output.json")
            if plot:
                durs, mapped_durs = generate_plot(tracepath, profile_filepath, outputpath, False)
                trace_durations += durs
                mapped_durations += mapped_durs
            else:
                generate_trace(tracepath, profile_filepath, outputpath, unique_assignment, mode)
        
        if plot:
            #plt.plot(trace_durations, mapped_durations)
            fig = plt.figure()
            ax = fig.add_subplot(2, 1, 1)
            ax.scatter(trace_durations, mapped_durations)
            ax.set_xscale('log')
            plt.xlabel("Trace functions 50-th percentile duration")
            plt.ylabel("Mapped profile function 50-th percentile duration")
            plt.savefig('profiled_150.png')
    
    else:
        generate_trace(trace_directorypath, profile_filepath, output_filepath, unique_assignment, mode)

if __name__ == "__main__":
    main()