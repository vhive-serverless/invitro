import numpy as np
import scipy.optimize as sp
import math

from log_config import *
from typing import Tuple

def get_error(trace_function, proxy_function) -> float:
    """
    Returns a float value on how close the trace function is to the proxy function. Lower the value, better the correlation.
    Euclidean distance between normalized memory and duration is considered.

    Parameters:
    - `trace_function` (dict): Dictionary containing information regarding trace function
    - `proxy_function` (dict): Dictionary containing information regarding proxy function

    Returns:
    - `float`: closeness value
    """

    try:
        trace_memory = trace_function["memory"]["50-percentile"]
        proxy_memory = proxy_function["memory"]["50-percentile"]
        trace_duration = trace_function["duration"]["50-percentile"]
        proxy_duration = proxy_function["duration"]["50-percentile"]
    except KeyError as e:
        log.warning(f"Correlation cannot be found. Error: {e}")
        return math.inf

    # NOTE: Better Error mechanisms can be considered to improve the correlation
    # Currently only the 50%tile memory and duration are considered.
    # Euclidean distance between normalized memory and duration is considered
    try:
        if trace_memory == 0: trace_memory += 0.01
        if trace_duration == 0: trace_duration += 0.01
        diff_memory = (math.log(trace_memory) - math.log(proxy_memory)) 
        diff_duration = (math.log(trace_duration) - math.log(proxy_duration)) 
        error = math.sqrt((diff_memory) ** 2 + (diff_duration) ** 2)
        return error
    except ValueError as e:
        log.warning(f"Correlation cannot be found. Error: {e}")
        return math.inf


def get_closest_proxy_function(
    trace_functions: dict, proxy_functions: dict
) -> Tuple[dict, int]:
    """
    Obtains the closest proxy function for every trace function
    
    Parameters:
    - `trace_functions` (dict): Dictionary containing information regarding trace functions
    - `proxy_functions` (dict): Dictionary containing information regarding proxy functions
    
    Returns:
    - `dict`: Dictionary containing information regarding trace functions with the associated proxy functions
    - `int`: 0 if no error. -1 if error
    """

    try:
        proxy_list = []
        for function_name in proxy_functions:
            proxy_list.append(proxy_functions[function_name])
            proxy_functions[function_name]["index"] = len(proxy_list) - 1

        for function_name in trace_functions:
            min_error = math.inf
            min_error_index = -1
            for i in range(0, len(proxy_list)):
                error = get_error(trace_functions[function_name], proxy_list[i])
                if error < min_error:
                    min_error = error
                    min_error_index = i

            if min_error == math.inf:
                log.warning(f"Proxy function for function {function_name} not found")
                continue

            trace_functions[function_name]["proxy-function"] = proxy_list[
                min_error_index
            ]["name"]
            trace_functions[function_name]["proxy-correlation"] = get_error(
                trace_functions[function_name], proxy_list[min_error_index]
            )
            log.debug(
                f"Found proxy function for {function_name}: {trace_functions[function_name]['proxy-function']} with correlation: {trace_functions[function_name]['proxy-correlation']}"
            )

        for function_name in proxy_functions:
            del proxy_functions[function_name]["index"]

        return trace_functions, 0

    except Exception as e:
        log.error(f"Finding closest proxy function failed. Error: {e}")
        return trace_functions, -1


def get_proxy_function(
    trace_functions: dict, proxy_functions: dict
) -> Tuple[dict, int]:
    """
    Obtains the closest proxy function for every trace function
    
    Parameters:
    - `trace_functions` (dict): Dictionary containing information regarding trace functions
    - `proxy_functions` (dict): Dictionary containing information regarding proxy functions
    
    Returns:
    - `dict`: Dictionary containing information regarding trace functions with the associated proxy functions
    - `int`: 0 if no error. -1 if error
    """

    log.info(
        f"Lower the correlation value, the proxy function is a better proxy of the trace function"
    )

    log.info(
        f"Getting closest proxy function for every trace function."
    )
    trace_functions, err = get_closest_proxy_function(
        trace_functions=trace_functions, proxy_functions=proxy_functions
    )

    if err == -1:
        log.critical(f"Mapping between trace function and proxy function not obtained")
        return trace_functions, -1

    return trace_functions, 0