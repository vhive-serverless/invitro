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
#  SOFTWARE.

from pathlib import Path

import pandas as pd
import numpy as np
import pytest

from sampler.preprocess2021 import (
    preprocess_file,
    filter_within_time_interval,
    filter_functions_with_0ms_inovcations,
    indicate_functions_with_0ms,
    transform_to_sampler_format,
    generate_inv_df,
    generate_mem_df,
    generate_dur_df,
)

# Validate [,) interval slicing
def test_filter_within_time_interval():
    input_df = pd.DataFrame({
        "app":           ["app1", "app2", "app3", "app4"],
        "func":          [  "f1",   "f1",   "f2",   "f3"],
        "end_timestamp": [  45.0,   70.0,   95.0,  121.0],
        "duration":      [  30.0,   10.0,   20.0,    1.0],
        #"start_timestamp":[15, 60, 75, 120],
    })

    expected_df = pd.DataFrame({
        "app":           ["app2", "app3"],
        "func":          [  "f1",   "f2"],
        "end_timestamp": [  70.0,   95.0],
        "duration":      [  10.0,   20.0],
        "start_timestamp": [60, 75],
    })
    expected_df['start_timestamp'] = pd.to_timedelta(expected_df['start_timestamp'], unit='s')

    # Interval of [60, 120)
    day_hour_minutes = "00:00:01"
    duration_minutes = "1"

    time_filtered_df, interval_start, interval_end = filter_within_time_interval(input_df, day_hour_minutes, duration_minutes)

    assert interval_start == pd.Timedelta(minutes=1)
    assert interval_end == pd.Timedelta(minutes=2)
    pd.testing.assert_frame_equal(time_filtered_df.reset_index(drop=True), expected_df.reset_index(drop=True))


# Validate 0ms indicating and counting
def test_indicate_functions_with_0ms():
    row = pd.Series({"duration": [0, 0, 10]})
    result = indicate_functions_with_0ms(row, threshold_percent=50)

    assert result["filter_out"] == 1
    assert result["total_invocation_count"] == 3
    
    row = pd.Series({"duration": [0, 5, 10, 15, 20]})
    result = indicate_functions_with_0ms(row, threshold_percent=75)

    assert result["filter_out"] == 0
    assert result["total_invocation_count"] == 5


# Bin column generation, invocation binning.
def test_generate_inv_df_bins():
    input_df = pd.DataFrame({
        "HashApp":          ["aa", "ab", "ab"],
        "HashFunction":     ["fa", "fb", "fc"],
        "start_timestamp":  [
            [pd.Timedelta(seconds=60)], 
            [pd.Timedelta(seconds=90)], 
            [pd.Timedelta(seconds=120), pd.Timedelta(seconds=150), pd.Timedelta(seconds=180)]
        ],
        "duration":         [
            [10.0],
            [20.0],   
            [30.0, 50.0, 70.0],
        ]
    })

    expected_df = pd.DataFrame(
        {
            "HashOwner":    [   0,    0,    0],
            "HashApp":      ["aa", "ab", "ab"],
            "HashFunction": ["fa", "fb", "fc"],
            "Trigger":      ["http", "http", "http"],
            2:              [1, 1, 0],
            3:              [0, 0, 2],
            4:              [0, 0, 1],
        }
    )
    inv_df = generate_inv_df(input_df, start_minute_bin=2, end_minute_bin=5)

    pd.testing.assert_frame_equal(inv_df, expected_df)


def test_generate_mem_df_fills_static_memory_values():
    input_df = pd.DataFrame({
        "HashApp":          ["aa", "ab", "ab"],
        "HashFunction":     ["fa", "fb", "fc"],
        "start_timestamp":  [
            [pd.Timedelta(seconds=60)], 
            [pd.Timedelta(seconds=90)], 
            [pd.Timedelta(seconds=120), pd.Timedelta(seconds=150), pd.Timedelta(seconds=180)]
        ],
        "duration":         [
            [10.0],
            [20.0],   
            [30.0, 50.0, 70.0],
        ]
    })

    static_value = 200.0
    expected_df = pd.DataFrame(
        {
            "HashFunction": ["fa", "fb", "fc"],
            "HashOwner":    [   0,    0,    0],
            "HashApp":      ["aa", "ab", "ab"],
            "SampleCount":  [   1,    1,    3],
            "AverageAllocatedMb":        [static_value, static_value, static_value],
            "AverageAllocatedMb_pct1":   [static_value, static_value, static_value],
            "AverageAllocatedMb_pct5":   [static_value, static_value, static_value],
            "AverageAllocatedMb_pct25":  [static_value, static_value, static_value],
            "AverageAllocatedMb_pct50":  [static_value, static_value, static_value],
            "AverageAllocatedMb_pct75":  [static_value, static_value, static_value],
            "AverageAllocatedMb_pct95":  [static_value, static_value, static_value],
            "AverageAllocatedMb_pct99":  [static_value, static_value, static_value],
            "AverageAllocatedMb_pct100": [static_value, static_value, static_value],
        }
    )

    mem_df = generate_mem_df(input_df)

    pd.testing.assert_frame_equal(mem_df, expected_df)

# Unit happy path test.
def test_preprocess2021(tmp_path):

    # Original DF
    og_df = pd.DataFrame(
        {
            "app":           ["aa", "ab", "ac",  "ac",  "ac"],
            "func":          ["fa", "fb", "fc",  "fd",  "fd"],
            "end_timestamp": [1.00, 10.0, 80.0, 100.0, 300.0], # 5 Minutes, 0-300 seconds
            "duration":      [0.50,  5.0, 15.5,  40.0, 100.0],
        # "start_timestamp": [0.50,  5.0, 64.5,  60.0, 200.0]
        }
    )
    dir_path = tmp_path / "og_df"
    dir_path.mkdir()
    orig_trace_path = dir_path / "AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt"
    og_df.to_csv(orig_trace_path, index=False)

    # Test Preprocess2021
    out_dir = tmp_path / "output"
    start_time = "00:00:00"
    duration = 5
    zero_ms_threshold_percent = 50
    
    preprocess_file(orig_trace_path, start_time, str(duration), str(out_dir), str(zero_ms_threshold_percent))

    # Read and compare output preprocessed2021 (inv_df)
    preprocessed2021_df_path = out_dir / "invocations.csv"
    preprocessed2021_df = pd.read_csv(preprocessed2021_df_path)

    expected_preprocessed2021_df = pd.DataFrame(
        {
            "HashOwner":    [   0,    0,    0,    0],
            "HashApp":      ["aa", "ab", "ac", "ac"],
            "HashFunction": ["fa", "fb", "fc", "fd"],
            "Trigger":      ["http", "http", "http", "http"],
            "1":            [1, 1, 0, 0],
            "2":            [0, 0, 1, 1],
            "3":            [0, 0, 0, 0],
            "4":            [0, 0, 0, 1],
            "5":            [0, 0, 0, 0],
        }
    )

    pd.testing.assert_frame_equal(preprocessed2021_df, expected_preprocessed2021_df)

