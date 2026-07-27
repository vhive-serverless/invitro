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

def convert_ibm2026_to_azure2019(trace_dir: str, start_time: str, duration: str, output_dir: str):

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
    zero_offset_seconds = dataset_zero.total_seconds()
    start_sec_zeroed = (td_interval_start + dataset_zero).total_seconds()
    end_sec_zeroed = (td_interval_end + dataset_zero).total_seconds()

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
            # Keep only correctly zeroed timestamp
            filtered_inv.append((inv_arr[mask] - zero_offset_seconds).tolist())
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
    final_df = final_df.drop(columns=['NumEvents'])
    final_df = final_df.explode(['InvocationTimes', 'AppExecTimes'])
    final_df['end_timestamp'] = (pd.to_timedelta(final_df["InvocationTimes"], unit="s") + pd.to_timedelta(final_df["AppExecTimes"], unit="ms"))
    final_df['AppExecTimes'] = pd.to_timedelta(final_df["AppExecTimes"], unit="ms").dt.total_seconds()

    # Start trace sample from 0
    final_df['end_timestamp'] = (final_df['end_timestamp'] - td_interval_start).dt.total_seconds()

    # Rename + Sort
    final_df = final_df.rename(columns={'NamespaceHash': 'app', 'AppHash': 'func', 'AppExecTimes': 'duration'})
    column_order = ["app", "func", "end_timestamp", "duration"]
    final_df = final_df.reindex(columns=column_order)
    final_df = final_df.sort_values(by='end_timestamp')

    # Save to output
    output_dir = Path(output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)
    final_df.to_csv(output_dir / "IBM2026AsAzure2021.csv", index=False)

    # Save app_configs
    file_path = Path(trace_dir / f"app_configs.pickle")
    app_config_df = pd.read_pickle(file_path)
    app_config_df.to_csv(output_dir / "app_configs.csv", index=False)

if __name__ == "__main__":
    
    trace_dir: str = r"data\traces\pickle_data"
    start_time: str = r"00:01:00"
    duration_minutes: str = r"60"
    output_dir: str = r"data\traces\output"

    convert_ibm2026_to_azure2019(trace_dir, start_time, duration_minutes, output_dir)