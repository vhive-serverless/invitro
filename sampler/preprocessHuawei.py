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


if __name__ == "__main__":

    trace_dir = "..\Huawei2023\private_dataset"
    start_time = "00:00:30"  # DD:HH:MM 
    duration = 5             # Minutes
    output_dir = "..\Huawei2023\output"
    
    preprocess_huawei(trace_dir, start_time, duration, output_dir, 0)
