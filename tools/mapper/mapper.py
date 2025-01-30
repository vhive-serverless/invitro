import os

import json
import re
import argparse
import pandas as pd
from find_proxy_function import *

from log_config import *

INVOCATION_COLUMN = 4
VSWARM_MAX_DUR = 27000

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
    else:
        log.critical(f"Durations file {duration_filepath} not found")
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
    else:
        log.critical(f"Memory file {memory_filepath} not found")
        return None, -1
        
    # Rename all columns in the dataframe with a lambda (for example: percentile_Average_1 -> 1-percentile) if x matches a regex
    duration_info = duration_info.rename(columns=lambda x: re.sub(r'percentile_(\w+)_(\d+)', r'\2-' + "percentile", x))
    # Rename all columns in the dataframe with a lambda (for example: AverageAllocatedMb_pct1 -> 1-percentile) if x matches a regex
    memory_info = memory_info.rename(columns=lambda x: re.sub(r'AverageAllocatedMb_pct(\w+)', r'\1-' + "percentile", x))

    # Add them to a dict with the key being the HashFunction
    trace_functions = {}

    for _, row in duration_info.iterrows():
        unique_id = row["HashFunction"] + row["HashOwner"] + row["HashApp"]
        trace_functions[unique_id] = {}
        trace_functions[unique_id]["duration"] = row

    for _, row in memory_info.iterrows():
        unique_id = row["HashFunction"] + row["HashOwner"] + row["HashApp"]
        trace_functions[unique_id]["memory"] = row

    return trace_functions, 0

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
        log.critical(f"Trace loading failed")
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
    trace_functions = get_closest_proxy_function(
        trace_functions=trace_functions,
        proxy_functions=proxy_functions,
    )
    
    log.info(f"Proxy functions obtained")

    # Writing the proxy functions to a file

    # Only give function name and proxy name
    trace_json = {}
    for id in trace_functions:
        trace_json[id] = {}
        trace_json[id]["proxy-function"] = trace_functions[id]["proxy-function"] 

    try:
        with open(output_filepath, "w") as jf:
            json.dump(trace_json, jf, indent=4)
    except Exception as e:
        log.critical(f"Output file {output_filepath} cannot be written. Error: {e}")
        log.critical(f"Load Generation failed")
        return
    
    log.info(f"Output file {output_filepath} written")
    log.info(f"Load Generation successful. Mapper output generated.")

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
    for id in trace_functions:
        if trace_functions[id]["proxy-function"] == "trace-func-go":
            dur_count += 1
            continue
        trace_mem = trace_functions[id]["memory"]["50-percentile"]
        trace_dur = trace_functions[id]["duration"]["50-percentile"]
        proxy_dur = proxy_functions[trace_functions[id]["proxy-function"]]["duration"]["50-percentile"]
        proxy_mem = proxy_functions[trace_functions[id]["proxy-function"]]["memory"]["50-percentile"]
        mem_error += trace_mem - proxy_mem
        rel_mem_error += (trace_mem - proxy_mem)/trace_mem
        abs_mem_error += abs(trace_mem - proxy_mem)
        abs_rel_mem_error += abs((trace_mem - proxy_mem)/trace_mem)
        dur_error += trace_dur - proxy_dur
        abs_dur_error += abs(trace_dur - proxy_dur)
        if trace_dur == 0:
            zero_duration += 1
        else:
            rel_dur_error += (trace_dur - proxy_dur)/trace_dur
            abs_rel_dur_error += abs((trace_dur - proxy_dur)/trace_dur)

    total_functions = len(trace_functions)
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

if __name__ == "__main__":
    main()