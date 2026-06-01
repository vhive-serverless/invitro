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

from numpy import s_ 
import pandas as pd
import logging as log
import numpy as np

from glob import glob
from tqdm import tqdm
from typing import Tuple
import math

import os

def preprocess_file(trace_dir: str, start_time: str, duration: str, output_dir: str) -> pd.DataFrame:
    
    # Read CSV
    trace_file = glob(f"{trace_dir}/AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt")
    assert len(trace_file) == 1, "There exists only 1 Azure2021 trace file"
    df = pd.read_csv(trace_file[0])

    # Keep invocations within interval
    df, start_td, end_td = time_slice(df, start_time, duration)

    # TODO: Determine if zero duration keep or throw? (see if loader parses correctly.)

    # Validate no NaN
    df = df.dropna()
    assert df.isna().sum().sum() == 0

    # Transform into sampler's expected format.
    inv_df, mem_df, dur_df = transform_to_sampler_format(df, start_td, end_td)

    # Save to directory
    if not os.path.exists(output_dir):
        try:
            os.makedirs(output_dir)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")

    inv_df.to_csv(f"{output_dir}/invocations.csv", index=False)
    mem_df.to_csv(f"{output_dir}/memory.csv", index=False)
    dur_df.to_csv(f"{output_dir}/durations.csv", index=False)

def time_slice(df: pd.DataFrame, start_time: str, duration: str) -> Tuple[pd.DataFrame, pd.Timedelta, pd.Timedelta]:
    
    # Generate start time
    df['start_timestamp'] = df['end_timestamp'] - df['duration']
    df['start_timestamp'] = pd.to_timedelta(df['start_timestamp'], unit='s')

    # Determine time interval
    start_time = start_time.split(":")
    day = int(start_time[0])
    hours = int(start_time[1])
    minutes = int(start_time[2])
    duration = int(duration)

    interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration))

    # Get time interval slice
    df = df[df['start_timestamp'].between(interval_start, interval_end)]

    return df, interval_start, interval_end

def transform_to_sampler_format(df: pd.DataFrame, interval_start: pd.Timedelta, interval_end: pd.Timedelta
                                ) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    
    # Determine invocation minute bins
    start_minute = math.floor((interval_start.total_seconds() / 60))
    start_minute_bin = start_minute + 1 # Azure2019 starts from minute 1. (invocations in the first minute)
    
    end_minute = math.floor((interval_end.total_seconds() / 60))
    end_minute_bin = end_minute + 1

    # Group into per-func basis
    df = df.groupby(['app', 'func'])[['start_timestamp', 'duration']].agg(list).reset_index()
    df = df.rename(columns={'app': 'HashApp', 'func': 'HashFunction'})

    inv_df = generate_inv_df(df, start_minute_bin, end_minute_bin)
    mem_df = generate_mem_df(df)
    dur_df = generate_dur_df(df)

    return inv_df, mem_df, dur_df
    
def generate_inv_df(df: pd.DataFrame, start_minute_bin: int, end_minute_bin: int) -> pd.DataFrame:
    
    # INV_DF Determine invocation data in a per-minute bin-basis
    # Set new columns, add per-minute bins
    new_columns = ["HashOwner", "HashApp", "HashFunction", "Trigger"] + list(range(start_minute_bin, end_minute_bin)) + ["start_timestamp"]
    df = df.reindex(columns=new_columns, fill_value=0) 

    # Apply the function across rows (axis=1)  
    df = df.apply(bin_timestamps, axis=1)

    # Cleanup
    df = df.drop(columns=["start_timestamp"])
    df["Trigger"] = "http"
    df["HashOwner"] = 0

    return df

def bin_timestamps(row):
    # Parse timestamps, determine bin
    timestamp_list = row['start_timestamp']
    for timestamp in timestamp_list:
        timestamp_minute_bin = math.floor((timestamp.total_seconds() / 60)) + 1

        # Increment count 
        row[timestamp_minute_bin] += 1 
    return row

#TODO: Test
def generate_mem_df(df: pd.DataFrame) -> pd.DataFrame:
    new_columns = [
        "HashFunction", "HashOwner", "HashApp", "SampleCount", 
        "AverageAllocatedMb", "AverageAllocatedMb_pct1", "AverageAllocatedMb_pct5", "AverageAllocatedMb_pct25"
        "AverageAllocatedMb_pct50", "AverageAllocatedMb_pct75", "AverageAllocatedMb_pct95", "AverageAllocatedMb_pct99", "AverageAllocatedMb_pct100"
    ] + ["start_timestamp"]
    df = df.reindex(columns=new_columns) 

    df['SampleCount'] = df['start_timestamp'].str.len() #TODO: Determine if SampleCount needs a particular value, what reference value to use.
    df = df.fillna(200.0) # 200MB, based on average stated in Azure2019

    # Cleanup
    df = df.drop(columns=["start_timestamp"])
    df["HashOwner"] = 0

    return df

#TODO: Test
def generate_dur_df(df: pd.DataFrame) -> pd.DataFrame:
    new_columns = [
        "HashOwner", "HashApp", "HashFunction", 
        "Average", "Count", "Minimum", "Maximum",
        "percentile_Average_0", "percentile_Average_1", "percentile_Average_25", "percentile_Average_50", 
        "percentile_Average_75", "percentile_Average_99", "percentile_Average_100"
    ] + ["duration"]
    df = df.reindex(columns=new_columns)

    # Apply the function across rows (axis=1)  
    df = df.apply(generate_duration_statistics, axis=1)

    # Cleanup
    df = df.drop(columns=["duration"])
    df["HashOwner"] = 0

    return df

def generate_duration_statistics(row):
    timestamp_list = row['duration']

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


if __name__ == "__main__":
    trace_dir = "data/azure2021/"
    out_dir = "data/traces/reference/preprocessedAzure2021"
    start_time = "00:01:00"
    duration = "100"

    preprocess_file(trace_dir=trace_dir, start_time=start_time, duration=duration, output_dir=out_dir)

