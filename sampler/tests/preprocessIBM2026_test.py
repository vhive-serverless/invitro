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

from sampler.preprocessIBM2026 import (
    convert_to_azure2021,
    read_df_from_pickle_files
)

# What should unit tests test for? ->
# 
# Test reading from all pickles, obtaining a value.
# Test conversion from IBM2026 ideal to azure 2019.

def test_convert_to_azure2021():

    input_df = pd.DataFrame({
        "NamespaceHash":    ["0", "0", "1", "2"], # Test unique ID (namespace + app)
        "AppHash":          ["A", "B", "B", "C"], # Test unique ID (namespace + app)
        "NumEvents":        [1, 1, 2, 1],         # Test multiple events
        "InvocationTimes":  [[10], [20.5], [33, 47], [33]], # Test end_timestamp sorting
        "AppExecTimes":     [[3], [5], [10, 4], [5]] # ms
    })

    expected_df = pd.DataFrame({
        "app":           ["0", "0", "2", "1", "1"],
        "func":          ["A", "B", "C", "B", "B"],
        "end_timestamp": [10.003, 20.505, 33.005, 33.01, 47.004],
        "duration":      [0.003, 0.005, 0.005, 0.01, 0.004] # s
    })

    zero_td = pd.Timedelta(0, unit="s")
    azure2021_df = convert_to_azure2021(input_df, zero_td)
    pd.testing.assert_frame_equal(azure2021_df, expected_df)


## Assume dataset has uniform offset start at 1 hour
## Get invocations within 1 minute starting from 1 hr (Inovcations from 3600 -> 3660)
def test_read_df_from_single_pickle_file(tmp_path):

    trace_dir = tmp_path / "pickle_data"
    trace_dir.mkdir()

    # Week within interval
    input_df = pd.DataFrame({
        "NamespaceHash":    ["0", "0", "1", "2"], 
        "AppHash":          ["A", "B", "B", "C"], 
        "NumEvents":        [1, 1, 2, 1],         
        "InvocationTimes":  [[3500], [3620.5], [3633, 3647], [5000]], # s # Test invocation interval filter
        "AppExecTimes":     [[3], [5], [10, 4], [5]], # ms
        "TotalExecTimes":   [[10], [10], [15, 10], [10]], # ms
        "PodHash":          [['AAA'], ['BBB'], ['CCC', 'CCC'], ['DDD']]
    })
    input_df.to_pickle(trace_dir / "week_1.pickle")

    # Expected
    expected_df = pd.DataFrame({
        "NamespaceHash":    ["0", "1"], 
        "AppHash":          ["B", "B"], 
        "NumEvents":        [1, 2],         
        "InvocationTimes":  [[20.5], [33, 47]], 
        "AppExecTimes":     [[5], [10, 4]]
    })

    # Invocation Parameters
    day = 0
    hours = 0
    minutes = 0
    duration_minutes = 1
    dataset_zero = pd.Timedelta(hours=1, minutes=0, seconds=0)
    td_interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    td_interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration_minutes))

    ibm2026_df = read_df_from_pickle_files(trace_dir, td_interval_start, td_interval_end, dataset_zero)

    pd.testing.assert_frame_equal(ibm2026_df, expected_df)

# interval span both week 1 and 2
# seconds interval (603,000 -> 606,600)
def test_read_df_from_multiple_pickle_files(tmp_path):
    trace_dir = tmp_path / "pickle_data"
    trace_dir.mkdir()

    # Input
    week1_df = pd.DataFrame({
        "NamespaceHash":    ["0", "1"], 
        "AppHash":          ["A", "B"], 
        "NumEvents":        [1, 1],         
        "InvocationTimes":  [[602_000], [603_010]],
        "AppExecTimes":     [[10], [20]], 
        "TotalExecTimes":   [[100], [100]],
        "PodHash":          [['AAA'], ['AAA']]
    })
    week1_df.to_pickle(trace_dir / "week_1.pickle")

    week2_df = pd.DataFrame({
        "NamespaceHash":    ["2", "3"], 
        "AppHash":          ["B", "C"], 
        "NumEvents":        [1, 1],         
        "InvocationTimes":  [[606_500], [606_700]],
        "AppExecTimes":     [[3], [5]],
        "TotalExecTimes":   [[100], [100]],
        "PodHash":          [['AAA'], ['AAA']]
    })
    week2_df.to_pickle(trace_dir / "week_2.pickle")

    # Expected
    expected_df = pd.DataFrame({
        "NamespaceHash":    ["1", "2"], 
        "AppHash":          ["B", "B"], 
        "NumEvents":        [1, 1],         
        "InvocationTimes":  [[603_010], [606_500]], 
        "AppExecTimes":     [[20], [3]]
    })

    # Invocation Parameters
    day = 6
    hours = 23
    minutes = 30
    duration_minutes = 60
    dataset_zero = pd.Timedelta(hours=0, minutes=0, seconds=0)
    td_interval_start = pd.Timedelta(days=day, hours=hours, minutes=minutes)
    td_interval_end = pd.Timedelta(days=day, hours=hours, minutes=(minutes+duration_minutes))

    ibm2026_df = read_df_from_pickle_files(trace_dir, td_interval_start, td_interval_end, dataset_zero)

    pd.testing.assert_frame_equal(ibm2026_df, expected_df)