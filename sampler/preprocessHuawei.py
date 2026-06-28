#  MIT License
#
#  Copyright (c) 2026 HySCALE and vHive community
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

import pandas as pd
import logging as log
import numpy as np
from pathlib import Path
from typing import Tuple

def preprocess_huawei(trace_dir: str, start_time: str, duration: str, output_dir: str, zero_ms_threshold_percent: str) -> pd.DataFrame:
    
    # Time interval filter // Allow cross day filtering?
    start_time = start_time.split(":")
    day = int(start_time[0])
    hours = int(start_time[1])
    minutes = int(start_time[2])
    duration = int(duration)

    # Determine time interval
    td_interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    td_interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration))
    starting_day = td_interval_start.days
    ending_day = td_interval_end.days

    # Read all metrics within time interval
    metrics = {
        "function_delay_minute": {"path": Path("function_delay_minute"), "df": pd.DataFrame()},
        "memory_limit_minute": {"path": Path("memory_limit_minute"), "df": pd.DataFrame()},
        "memory_usage_minute": {"path": Path("memory_usage_minute"), "df": pd.DataFrame()},
        "requests_minute": {"path": Path("function_delay_minute"), "df": pd.DataFrame()},
    }
    for metric, value in metrics.items():
        directory = Path(trace_dir) / value["path"]
        final_df = pd.DataFrame()

        # Determine files to read
        for day in range(starting_day, ending_day + 1):
            file_path = directory / f"day_{day:03d}.csv" # Leading zeros, width of 3 (001, 002)
            df = pd.read_csv(file_path)

            # Filter by timestamp
            df = df[df["time"].between(td_interval_start.seconds, td_interval_end.seconds, inclusive='left')] # left <= series < right

            final_df = pd.concat([final_df, df], ignore_index=True)

        value["df"] = final_df

    # Transform to sampler format (inv_df, mem_df, run_df)
    # All generation filters out zero/NaN values
    inv_df = generate_inv_df(metrics["requests_minute"]["df"])
    mem_df = generate_mem_df(metrics["memory_limit_minute"]["df"])
    dur_df = generate_dur_df(metrics["function_delay_minute"]["df"])

    inv_df, mem_df, dur_df = get_intersection(inv_df, mem_df, dur_df)

    # Save to output
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    
    inv_df.to_csv(output_dir / "invocations.csv", index=False)
    mem_df.to_csv(output_dir / "memory.csv", index=False)
    dur_df.to_csv(output_dir / "durations.csv", index=False)
    
    return

# Count of invocations per minute. Filters out functions with 0 invocations.
def generate_inv_df(requests_minute_df: pd.DataFrame) -> pd.DataFrame:

    # Make columns into minute bins
    df = requests_minute_df.drop(columns='day')
    df['time'] = df['time']/60 + 1 # inv_df starts from minute 1
    df['time'] = df['time'].astype(int).astype(str) # 1.0 -> 1
    df = df.set_index('time', drop=True)
    df = df.T

    # Add in front 4 columns
    front_cols = ["HashOwner", "HashApp", "HashFunction", "Trigger"]
    empty_front_df = pd.DataFrame(columns=front_cols, index=df.index)
    df = pd.concat([empty_front_df, df], axis=1)

    df["HashOwner"] = "0"
    df['HashApp'] = df.index
    df['HashFunction'] = df.index
    df["Trigger"] = "http"

    # Filter out functions with 0 invocations
    prefiltered_df = df

    minute_bin_columns = df.columns[4:]
    df = df.dropna(subset=minute_bin_columns, how='all')

    log.info(f"Inv df removed uninvoked functions (before -> after): {len(prefiltered_df)} -> {len(df)}")

    # Set 0 invocations from NaN to 0.
    df = df.fillna(0)
    df = df.rename_axis(None, axis=1)
    df = df.reset_index(drop=True)
    df[minute_bin_columns] = df[minute_bin_columns].astype(np.int64)

    return df

# Memory is total function footprint -> allocated memory across all pods for a single function.
def generate_mem_df(memory_limit_minute: pd.DataFrame) -> pd.DataFrame:
    
    # Make columns into minute bins
    df = memory_limit_minute.drop(columns='day')
    df['time'] = df['time']/60 + 1 # inv_df starts from minute 1
    df['time'] = df['time'].astype(int).astype(str) # 1.0 -> 1
    df = df.set_index('time', drop=True)
    df = df.T

    minute_bin_columns = df.columns
    min_bin_df = df[minute_bin_columns]

    # Set IDs
    df["HashFunction"] = df.index
    df["HashOwner"] = "0"
    df['HashApp'] = df.index

    # Sample count is estimated as count of non-NAN samples
    df["SampleCount"] = min_bin_df.count(axis=1)

    # Calculate percentiles from non-NAN datapoints within time interval
    df["AverageAllocatedMb"] = min_bin_df.mean(axis=1)
    df["AverageAllocatedMb_pct1"] = min_bin_df.quantile(0.01, axis=1)
    df["AverageAllocatedMb_pct5"] = min_bin_df.quantile(0.05, axis=1)
    df["AverageAllocatedMb_pct25"] = min_bin_df.quantile(0.25, axis=1)
    df["AverageAllocatedMb_pct50"] = min_bin_df.quantile(0.50, axis=1)
    df["AverageAllocatedMb_pct75"] = min_bin_df.quantile(0.75, axis=1)
    df["AverageAllocatedMb_pct95"] = min_bin_df.quantile(0.95, axis=1)
    df["AverageAllocatedMb_pct99"] = min_bin_df.quantile(0.99, axis=1)
    df["AverageAllocatedMb_pct100"] = min_bin_df.quantile(1.00, axis=1)

    # Filter out zero allocated memory
    df = df.loc[df["SampleCount"] != 0]

    # Cleanup - Keep only required columns
    column_order = [
        "HashFunction", "HashOwner", "HashApp", "SampleCount", 
        "AverageAllocatedMb", "AverageAllocatedMb_pct1", "AverageAllocatedMb_pct5", "AverageAllocatedMb_pct25",
        "AverageAllocatedMb_pct50", "AverageAllocatedMb_pct75", "AverageAllocatedMb_pct95", "AverageAllocatedMb_pct99", "AverageAllocatedMb_pct100"
    ]
    df = df.reindex(columns=column_order)
    df = df.rename_axis(None, axis=1)
    df = df.reset_index(drop=True)

    return df

# Duration is function execution time averaged over all pods, timestamped in minute basis
def generate_dur_df(function_delay_minute: pd.DataFrame) -> pd.DataFrame:

    # Make columns into minute bins
    df = function_delay_minute.drop(columns='day')
    df['time'] = df['time']/60 + 1 # inv_df starts from minute 1
    df['time'] = df['time'].astype(int).astype(str) # 1.0 -> 1
    df = df.set_index('time', drop=True)
    df = df.T

    minute_bin_columns = df.columns
    min_bin_df = df[minute_bin_columns]

    # Set IDs
    df["HashOwner"] = "0"
    df['HashApp'] = df.index
    df["HashFunction"] = df.index

    # Generate stats (derived from datapoints within time interval)
    df["Average"] = min_bin_df.mean(axis=1)
    df["Count"] = min_bin_df.count(axis=1)
    df["Minimum"] = min_bin_df.min(axis=1)
    df["Maximum"] = min_bin_df.max(axis=1)
    df["percentile_Average_0"] = min_bin_df.quantile(0.00, axis=1)
    df["percentile_Average_1"] = min_bin_df.quantile(0.01, axis=1)
    df["percentile_Average_25"] = min_bin_df.quantile(0.25, axis=1)
    df["percentile_Average_50"] = min_bin_df.quantile(0.50, axis=1)
    df["percentile_Average_75"] = min_bin_df.quantile(0.75, axis=1)
    df["percentile_Average_99"] = min_bin_df.quantile(0.99, axis=1)
    df["percentile_Average_100"] = min_bin_df.quantile(1.00, axis=1)

    # Filter out zero duration
    df = df.loc[df["Count"] != 0]

    # Cleanup - Keep only required columns
    new_columns = [
        "HashFunction", "HashOwner", "HashApp",  
        "Average", "Count", "Minimum", "Maximum",
        "percentile_Average_0", "percentile_Average_1", "percentile_Average_25", "percentile_Average_50", 
        "percentile_Average_75", "percentile_Average_99", "percentile_Average_100"
    ]
    df = df.reindex(columns=new_columns)
    df = df.rename_axis(None, axis=1)
    df = df.reset_index(drop=True)

    return df

# Filter for functions that appear in all 3 dfs.
def get_intersection(
        inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    
    # Matches cols that are same across all 3 DFs
    cols = ['HashApp', 'HashFunction']
    common_idx = (
        inv_df.set_index(cols).index
        .intersection(mem_df.set_index(cols).index)
        .intersection(run_df.set_index(cols).index)
    )

    inv_df_cleaned = inv_df.set_index(cols).loc[common_idx].reset_index()
    mem_df_cleaned = mem_df.set_index(cols).loc[common_idx].reset_index()
    run_df_cleaned = run_df.set_index(cols).loc[common_idx].reset_index()

    log.info(f"inv_df row count after intersection: {len(inv_df)}")
    log.info(f"mem_df row count after intersection: {len(mem_df)}")
    log.info(f"run_df row count after intersection: {len(run_df)}")
    
    return inv_df_cleaned, mem_df_cleaned, run_df_cleaned

    # Leaves only rows with the common HashApp and HashFunction values in all 3 dfs

if __name__ == "__main__":

    trace_dir = "../Huawei2023/private_dataset"
    start_time = "00:00:30"  # DD:HH:MM 
    duration = 5             # Minutes
    output_dir = "../Huawei2023/output"
    
    preprocess_huawei(trace_dir, start_time, duration, output_dir, 0)
