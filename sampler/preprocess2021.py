#  MIT License
#
#  Copyright (c) 2023 EASL and the vHive community
#
#  Permission is hereby granted, free of charge, to any person obtaining a copy
#  of this software and associated documentation files (the "Software"), to deal
#  in the Software without restriction, including without limitation the rights
#  to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
#  copies of the Software, and to permit persons to whom the Software is
#  furnished to do so, subject to the following conditions:
#
#  The above copyright notice and this permission notice shall be included in all
#  copies or substantial portions of the Software.
#
#  THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
#  IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
#  FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
#  AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
#  LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
#  OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
#  SOFTWARE.

from cycler import L
from numpy import dsplit, s_ 
import pandas as pd
import logging as log
import numpy as np

from glob import glob
from tqdm import tqdm
from typing import Tuple
import math

import os

def preprocess_file(trace_dir: str, start_time: str, duration: str, output_dir: str, zero_ms_threshold_percent:str) -> pd.DataFrame:
    
    # Read CSV
    trace_file = glob(f"{trace_dir}/AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt")
    assert len(trace_file) == 1, "There exists only 1 Azure2021 trace file"
    df = pd.read_csv(trace_file[0])

    # Time interval filter
    df, start_td, end_td = filter_within_time_interval(df, start_time, duration)

    # Filter functions with invocations below 1ms
    df = filter_functions_with_0ms_inovcations(df, threshold_percent=int(zero_ms_threshold_percent))

    # Validation
    df = df.dropna()
    assert df.isna().sum().sum() == 0

    # Transform format
    inv_df, mem_df, dur_df = transform_to_sampler_format(df, start_td, end_td)

    log.info(f"The final dfs contain rows: "
             f"inv={len(inv_df)}, mem={len(mem_df)}, dur={len(dur_df)}")
    
    # Save to directory
    if not os.path.exists(output_dir):
        try:
            os.makedirs(output_dir)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")
        
    log.info(f"Saving dfs to {output_dir}")
    inv_df.to_csv(f"{output_dir}/invocations.csv", index=False)
    mem_df.to_csv(f"{output_dir}/memory.csv", index=False)
    dur_df.to_csv(f"{output_dir}/durations.csv", index=False)

# Filter for invocation from [start_time, start_time+duration_mintues)
def filter_within_time_interval(df: pd.DataFrame, start_time: str, duration_minutes: str) -> Tuple[pd.DataFrame, pd.Timedelta, pd.Timedelta]:
    
    # Generate start time
    df['start_timestamp'] = df['end_timestamp'] - df['duration']
    df['start_timestamp'] = pd.to_timedelta(df['start_timestamp'], unit='s')

    # Determine time interval
    start_time = start_time.split(":")
    day = int(start_time[0])
    hours = int(start_time[1])
    minutes = int(start_time[2])
    duration = int(duration_minutes)

    interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration))

    # Get time interval slice
    df = df[df['start_timestamp'].between(interval_start, interval_end, inclusive="left")]

    if (interval_end > pd.Timedelta(days=14, hours=0, minutes=0)):
        log.warning(f"interval_end includes time after 14 days. azure2021 only has 14 days of invocations, ensure start_time and duration entered is intended.")

    log.info(f"The time interval contains: {len(df)} invocations and {len(df.groupby(['app', 'func']))} functions")

    return df, interval_start, interval_end

# Remove functions with invocation of 0ms above threshold rate.
# This is to keep in line with preprocess.py's remove_zero_duration() which "Removes functions with an average execution time of 0 ms"
def filter_functions_with_0ms_inovcations(df: pd.DataFrame, threshold_percent: int):

    duration_grouped_df = df.groupby(['app', 'func'])['duration'].agg(list).reset_index()

    # Indicate which functions have 0ms invocations above threshold percent.
    duration_grouped_df = duration_grouped_df.apply(indicate_functions_with_0ms, threshold_percent=threshold_percent, axis=1)

    # Display to user count/percentage of functions/invocations removed
    total_functions_count = len(duration_grouped_df)
    total_invocation_count = duration_grouped_df['total_invocation_count'].sum()

    functions_to_remove = duration_grouped_df[duration_grouped_df['filter_out'] == 1]
    functions_removed_count = len(functions_to_remove)
    invocation_removed_count = functions_to_remove['total_invocation_count'].sum()

    log.info(f"Removing functions with 0ms invocations rate above {threshold_percent}%:")
    log.info(f"Removing {functions_removed_count} of {total_functions_count} functions ({(functions_removed_count/total_functions_count):.2%})")
    log.info(f"Removing {invocation_removed_count} of {total_invocation_count} invocations ({(invocation_removed_count/total_invocation_count):.2%})")

    # Keep in original df, the rows with ['app','func'] that are below threshold.
    functions_to_keep = duration_grouped_df[duration_grouped_df['filter_out'] == 0]
    valid_keys = functions_to_keep[['app', 'func']].drop_duplicates()
    # Inner join
    df = df.merge(valid_keys, on=['app', 'func'], how='inner')

    return df
    
def indicate_functions_with_0ms(row, threshold_percent):
    duration_list = row['duration']
    zero_invocation_count = duration_list.count(0)
    total_invocation_count = len(duration_list)

    if (zero_invocation_count/total_invocation_count >= threshold_percent/100):
        row['filter_out'] = 1
        row['total_invocation_count'] = total_invocation_count
        log.debug(f"Row with 0ms_invocation above threshold \n{row}")
    else: 
        row['filter_out'] = 0
        row['total_invocation_count'] = total_invocation_count

    return row

def transform_to_sampler_format(df: pd.DataFrame, interval_start: pd.Timedelta, interval_end: pd.Timedelta
                                ) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    
    # Determine invocation minute bins
    start_minute = math.floor((interval_start.total_seconds() / 60))
    start_minute_bin = start_minute + 1 # Azure2019 tracks invocations within Xth minute.
    
    end_minute = math.floor((interval_end.total_seconds() / 60))
    end_minute_bin = end_minute + 1

    # Group into per-func basis
    df = df.groupby(['app', 'func'])[['start_timestamp', 'duration']].agg(list).reset_index()
    df = df.rename(columns={'app': 'HashApp', 'func': 'HashFunction'})

    inv_df = generate_inv_df(df, start_minute_bin, end_minute_bin)
    mem_df = generate_mem_df(df)
    dur_df = generate_dur_df(df)

    return inv_df, mem_df, dur_df

# Determine invocation count in a per-minute bin basis
def generate_inv_df(df: pd.DataFrame, start_minute_bin: int, end_minute_bin: int) -> pd.DataFrame:
    
    # Add per-minute bins columns
    new_columns = ["HashOwner", "HashApp", "HashFunction", "Trigger"] + list(range(start_minute_bin, end_minute_bin)) + ["start_timestamp"]
    df = df.reindex(columns=new_columns, fill_value=0) 

    df = df.apply(bin_timestamps, axis=1)

    # Cleanup
    df = df.drop(columns=["start_timestamp"])
    df["Trigger"] = "http"
    df["HashOwner"] = 0

    return df

# Bin timestamps to minute bins
def bin_timestamps(row):
    timestamp_list = row['start_timestamp']
    for timestamp in timestamp_list:
        timestamp_minute_bin = math.floor((timestamp.total_seconds() / 60)) + 1

        row[timestamp_minute_bin] += 1 
    return row

# Memory usage percentiles set as static 200 value
def generate_mem_df(df: pd.DataFrame) -> pd.DataFrame:
    static_value = 200.0

    log.info(f"Generating mem_df using static memory value of {static_value}")

    new_columns = [
        "HashFunction", "HashOwner", "HashApp", "SampleCount", 
        "AverageAllocatedMb", "AverageAllocatedMb_pct1", "AverageAllocatedMb_pct5", "AverageAllocatedMb_pct25",
        "AverageAllocatedMb_pct50", "AverageAllocatedMb_pct75", "AverageAllocatedMb_pct95", "AverageAllocatedMb_pct99", "AverageAllocatedMb_pct100"
    ] + ["start_timestamp"]
    df = df.reindex(columns=new_columns) 

    df['SampleCount'] = df['start_timestamp'].str.len()
    df = df.fillna(static_value) # 200MB, based on average stated in Azure2019

    # Cleanup
    df = df.drop(columns=["start_timestamp"])
    df["HashOwner"] = 0

    return df

# Calculate statistics from duration list, convert s to ms.
def generate_dur_df(df: pd.DataFrame) -> pd.DataFrame:
    new_columns = [
        "HashOwner", "HashApp", "HashFunction", 
        "Average", "Count", "Minimum", "Maximum",
        "percentile_Average_0", "percentile_Average_1", "percentile_Average_25", "percentile_Average_50", 
        "percentile_Average_75", "percentile_Average_99", "percentile_Average_100"
    ] + ["duration"]
    df = df.reindex(columns=new_columns)

    df = df.apply(generate_duration_statistics, axis=1)

    # Cleanup
    df = df.drop(columns=["duration"])
    df["HashOwner"] = 0

    return df

def generate_duration_statistics(row):
    timestamp_list = row['duration']

    timestamp_list = [s * 1000 for s in timestamp_list] # duration is in ms

    row["Average"] = np.mean(timestamp_list)
    row["Count"] = len(timestamp_list)
    row["Minimum"] = np.min(timestamp_list)
    row["Maximum"] = np.max(timestamp_list)
    row["percentile_Average_0"] = np.percentile(timestamp_list, 0)
    row["percentile_Average_1"] = np.percentile(timestamp_list, 1)
    row["percentile_Average_25"] = np.percentile(timestamp_list, 25)
    row["percentile_Average_50"] = np.percentile(timestamp_list, 50)
    row["percentile_Average_75"] = np.percentile(timestamp_list, 75)
    row["percentile_Average_99"] = np.percentile(timestamp_list, 99)
    row["percentile_Average_100"] = np.percentile(timestamp_list, 100)

    return row