import os

import json
import argparse
from trace_parser import *
from find_proxy_function import *

from log_config import *

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
        "-o",
        "--output-filepath",
        type=str,
        help="Path to the output file",
        default="output.json",
        required=False,
    )
    parser.add_argument(
        "-u",
        "--unique-assignment",
        type=bool,
        help="Whether to assign unique proxy functions to each trace function",
        default=False,
        required=False,
    )
    args = parser.parse_args()
    trace_directorypath = args.trace_directorypath
    profile_filepath = args.profile_filepath
    output_filepath = args.output_filepath
    unique_assignment = args.unique_assignment
    trace_functions, err = load_trace(trace_directorypath)
    if err == -1:
        log.critical(f"Load Generation failed")
        return
    elif err == 0:
        log.info(f"Trace loaded")

    # Check whether the profile file for proxy functions exists or not
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
        # Function name here is the HashOwner + HashApp + HashFunction to be a unique identifier
        trace_json[function]["function_name"] = trace_functions[function]["name"]
        trace_json[function]["proxy-function"] = trace_functions[function]["proxy-function"]
        trace_json[function]["proxy-correlation"] = trace_functions[function]["proxy-correlation"]

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