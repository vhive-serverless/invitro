import os

import json
import re
import argparse
import pandas as pd
from find_proxy_function import *

from log_config import *

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
    args = parser.parse_args()
    trace_directorypath = args.trace_directorypath
    profile_filepath = args.profile_filepath
    output_filepath = trace_directorypath + "/mapper_output.json"
    unique_assignment = args.unique_assignment
    mode = args.mode
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

if __name__ == "__main__":
    main()