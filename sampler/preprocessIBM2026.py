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
from glob import glob

def convert_ibm2026_to_azure2019(trace_dir: str, start_time: str, duration: str):

    # Verify folder is correctly formatted
    ## Ensure folder exists
    trace_dir = Path(trace_dir)
    assert trace_dir.exists(), "Trace directory does not exist"

    ## Ensure all 11 pickle files present (app_config + week 1-10)
    expected_files = ["app_configs.pickle"] + [f"week_{i}.pickle" for i in range(1, 11)]

    for file in expected_files:
        file = Path(trace_dir / file)
        assert file.exists(), f"Missing expected pickle file: {file.name}"

    # Determine time interval
    start_time = start_time.split(":")
    day = int(start_time[0])
    hours = int(start_time[1])
    minutes = int(start_time[2])
    duration = int(duration)

    td_interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    td_interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration))

    ## Dataset has timestamp zero offset at 4:59:59
    dataset_zero = pd.Timedelta(hours=4, minutes=59, seconds=59)
    td_interval_start_with_zero = td_interval_start + dataset_zero
    start_sec_zeroed = td_interval_start_with_zero.total_seconds()
    td_interval_end_with_zero = td_interval_end + dataset_zero
    end_sec_zeroed = td_interval_end_with_zero.total_seconds()

    final_df = pd.DataFrame()

    # Read Weekly Data Files
    for week in range(1, 11):

        # Skip if time interval not within week.
        week_index = week - 1
        time_interval = pd.Interval(td_interval_start, td_interval_end)
        file_time_interval = pd.Interval(left=pd.Timedelta(days=(week_index)*7), right=pd.Timedelta(days=(week_index+1)*7), closed='left')
        if (time_interval not in file_time_interval):
            continue

        # Read DF
        file_path = Path(trace_dir / f"week_{week}.pickle")
        df = pd.read_pickle(file_path)

        # Pre-clean unused columns
        df = df.drop(columns=['TotalExecTimes', 'PodHash']) 

        # Filter invocations within interval
        num_events = []
        filtered_inv, filtered_app = [], []
        for inv, app in zip(df['InvocationTimes'], df['AppExecTimes']):
            inv_arr = np.array(inv)

            # Vectorized NumPy mask
            mask = (inv_arr > start_sec_zeroed) & (inv_arr < end_sec_zeroed)
            
            num_events.append(mask.sum())
            filtered_inv.append(inv_arr[mask].tolist())
            filtered_app.append(np.array(app)[mask].tolist())
        df['NumEvents'] = num_events
        df['InvocationTimes'] = filtered_inv
        df['AppExecTimes'] = filtered_app

        # cleanup
        df = df[df['InvocationTimes'].str.len() > 0]

        # Combine Dataframes (Interval across multiple weeks)
        final_df = pd.concat([final_df, df], axis=0, ignore_index=True)

        ## Combine same applications
        final_df = final_df.groupby(['NamespaceHash', 'AppHash'], as_index=False).agg({
            'NumEvents': 'sum',        # mathematical sum
            'InvocationTimes': 'sum',  # concat lists
            'AppExecTimes': 'sum',     # concat lists
        })

        ## Sort InvocationTimes, ensuring AppExecTimes follow order
        sorted_inv = []
        sorted_app = []
        for inv, app in zip(final_df['InvocationTimes'], final_df['AppExecTimes']):
            inv_arr = np.array(inv)
            app_arr = np.array(app)

            sort_idx = np.argsort(inv_arr)

            sorted_inv.append(inv_arr[sort_idx].tolist())
            sorted_app.append(app_arr[sort_idx].tolist())
        final_df['InvocationTimes'] = sorted_inv
        final_df['AppExecTimes'] = sorted_app

    # Transform to azure2021 format
    # (conversion) Then Transform explode into per-invocation basis/Azure2021 format.


    # Save to output






# def preprocess_huawei(trace_dir: str, start_time: str, duration: str, output_dir: str) -> pd.DataFrame:
    
#     # Read CSVs
#     metrics_to_read = {
#         "function_delay_minute": {"path": Path("function_delay_minute"), "df": pd.DataFrame()},
#         "memory_limit_minute": {"path": Path("memory_limit_minute"), "df": pd.DataFrame()},
#         "requests_minute": {"path": Path("function_delay_minute"), "df": pd.DataFrame()},
#     }
#     metrics = read_all_trace_csv(trace_dir, start_time, duration, metrics_to_read)

#     # Transform to sampler format (inv_df, mem_df, run_df)

#     # Save to output
#     # output_dir = Path(output_dir)
#     # output_dir.mkdir(parents=True, exist_ok=True)
#     # inv_df.to_csv(output_dir / "invocations.csv", index=False)

#     return

# def read_all_trace_csv(trace_dir: str, start_time: str, duration: str, metrics: dict[str, dict[str, pd.DataFrame]]) -> dict[str, dict[str, pd.DataFrame]]:

#     # Time interval filter
#     start_time = start_time.split(":")
#     day = int(start_time[0])
#     hours = int(start_time[1])
#     minutes = int(start_time[2])
#     duration = int(duration)

#     # Determine time interval
#     td_interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
#     td_interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration))
#     starting_day = td_interval_start.days
#     ending_day = td_interval_end.days

#     # Read all metrics within time interval
#     for metric, value in metrics.items():
#         directory = Path(trace_dir) / value["path"]
#         final_df = pd.DataFrame()

#         # Determine files to read
#         for day in range(starting_day, ending_day + 1):
#             file_path = directory / f"day_{day:03d}.csv" # Leading zeros, width of 3 (001, 002)
#             df = pd.read_csv(file_path)

#             # Filter by timestamp
#             df = df[df["time"].between(td_interval_start.total_seconds(), td_interval_end.total_seconds(), inclusive='left')] # left <= series < right

#             final_df = pd.concat([final_df, df], ignore_index=True)

#         value["df"] = final_df

#     return metrics

if __name__ == "__main__":
    
    trace_dir: str = r"data\traces\pickle_data"
    start_time: str = r"00:01:00"
    duration_minutes: str = r"60"
    convert_ibm2026_to_azure2019(trace_dir, start_time, duration_minutes)