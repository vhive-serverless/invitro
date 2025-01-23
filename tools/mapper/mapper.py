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
            if duration > 27000:
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
            if duration > 27000:
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
    args = parser.parse_args()
    trace_directorypath = args.trace_directorypath
    profile_filepath = args.profile_filepath
    output_filepath = trace_directorypath + "/mapper_output.json"
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
    log.info(f"Preliminary load Generation successful. Checking error levels in mapping...")

    # Read the mapper output
    try:
        with open(output_filepath, "r") as jf:
            mapper_output = json.load(jf)
    except Exception as e:
        log.critical(f"Error in loading mapper output file {e}")
        return

    # Check the memory and duration errors

    dur_count = 0
    mem_error = 0
    dur_error = 0
    rel_mem_error = 0
    rel_dur_error = 0
    abs_mem_error = 0
    abs_dur_error = 0
    abs_rel_mem_error = 0
    abs_rel_dur_error = 0
    zero_duration = 0
    for function in mapper_output:
        trace_mem = trace_functions[function]["memory"]["50-percentile"]
        trace_dur = trace_functions[function]["duration"]["50-percentile"]
        proxy_dur = proxy_functions[mapper_output[function]["proxy-function"]]["duration"]["50-percentile"]
        proxy_mem = proxy_functions[mapper_output[function]["proxy-function"]]["memory"]["50-percentile"]
        log.warning(f"Memory error for function {function} is {abs(trace_mem - proxy_mem)} MB per invocation")
        mem_error += trace_mem - proxy_mem
        rel_mem_error += (trace_mem - proxy_mem)/trace_mem
        abs_mem_error += abs(trace_mem - proxy_mem)
        abs_rel_mem_error += abs((trace_mem - proxy_mem)/trace_mem)
        log.warning(f"Duration error for function {function} is {abs(trace_dur - proxy_dur)}ms per invocation")
        dur_error += trace_dur - proxy_dur
        abs_dur_error += abs(trace_dur - proxy_dur)
        if trace_dur == 0:
            zero_duration += 1
        else:
            rel_dur_error += (trace_dur - proxy_dur)/trace_dur
            abs_rel_dur_error += abs((trace_dur - proxy_dur)/trace_dur)
        if abs(trace_dur - proxy_dur) > 0.4*trace_dur:
            mapper_output[function]["proxy-function"] = "trace-func-go"
            dur_count += 1

    total_functions = len(mapper_output)
    log.info(f"Average memory error: {mem_error/total_functions} MB per invocation")
    log.info(f"Average duration error: {dur_error/total_functions} ms per invocation")
    log.info(f"Average absolute memory error: {abs_mem_error/total_functions} MB per invocation")
    log.info(f"Average absolute duration error: {abs_dur_error/total_functions} ms per invocation")
    log.info(f"Average relative memory error: {rel_mem_error/total_functions}")
    log.info(f"Average relative duration error: {rel_dur_error/total_functions}")
    log.info(f"Average absolute relative memory error: {abs_rel_mem_error/total_functions}")
    log.info(f"Average absolute relative duration error: {abs_rel_dur_error/total_functions}")
    log.info(f"Duration errors: {dur_count}")
    log.info(f"Functions with 0 duration: {zero_duration}")

    log.info(f"Replacing the functions with high duration error with invitro functions.")

    ## Write the updated mapper output

    try:
        with open(output_filepath, "w") as jf:
            json.dump(mapper_output, jf, indent=4)
            log.info(f"Updated output file {output_filepath} written. Load generated.")
    except Exception as e:
        log.critical(f"Output file {output_filepath} cannot be written. Error: {e}")
        log.critical(f"Load Generation failed")
        return

if __name__ == "__main__":
    main()