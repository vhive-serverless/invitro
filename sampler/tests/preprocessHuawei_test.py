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
import os

from sampler.preprocessHuawei import (
    preprocess_huawei,
    generate_inv_df,
    generate_mem_df,
    generate_dur_df
)

NaN = np.nan

def test_generate_inv_df():
    input_df = pd.DataFrame({
        "day":  [0, 0, 0],
        "time": [0, 60, 120],
        "0":    [10, 20, 30],       #
        "1":    [NaN, NaN, NaN],    # Zero invocations -> drop
        "2":    [NaN, 10, NaN],     # At least 1 invocation -> keep
        "3":    [5583, 3552, 4254], # 
        "4":    [200, 0, 20],       # '0' in input -> keep
    })

    expected_df = pd.DataFrame({
        "HashOwner":    ["0", "0", "0", "0"],
        "HashApp":      ["0", "2", "3", "4"],
        "HashFunction": ["0", "2", "3", "4"],
        "Trigger":      ["http", "http", "http", "http"],
        "1":            [10, 0, 5583, 200],
        "2":            [20, 10, 3552, 0],
        "3":            [30, 0, 4254, 20],
    })

    inv_df = generate_inv_df(input_df)
    pd.testing.assert_frame_equal(inv_df, expected_df)

def test_generate_mem_df():

    input_df = pd.DataFrame({
        "day":  [0, 0, 0],
        "time": [0, 60, 120],
        "0":    [400, 400, 400],    #
        "1":    [NaN, NaN, NaN],    # Zero memory allocation -> drop
        "2":    [NaN, 10, 20],      # At least 1 memory allocation -> keep
        "3":    [10, 50, 90],       #
    })

    expected_df = pd.DataFrame({
        "HashFunction":              ["0", "2", "3"],
        "HashOwner":                 ["0", "0", "0"],
        "HashApp":                   ["0", "2", "3"],
        "SampleCount":               [  3,   2,   3],
        "AverageAllocatedMb":        [400.0, 15.0, 50.0],
        "AverageAllocatedMb_pct1":   [400.0, 10.1, 10.8],
        "AverageAllocatedMb_pct5":   [400.0, 10.5, 14.0],
        "AverageAllocatedMb_pct25":  [400.0, 12.5, 30.0],
        "AverageAllocatedMb_pct50":  [400.0, 15.0, 50.0],
        "AverageAllocatedMb_pct75":  [400.0, 17.5, 70.0],
        "AverageAllocatedMb_pct95":  [400.0, 19.5, 86.0],
        "AverageAllocatedMb_pct99":  [400.0, 19.9, 89.2],
        "AverageAllocatedMb_pct100": [400.0, 20.0, 90.0],
    })

    mem_df = generate_mem_df(input_df)
    pd.testing.assert_frame_equal(mem_df, expected_df)

def test_generate_dur_df():

    input_df = pd.DataFrame({
        "day":  [0, 0, 0],
        "time": [0, 60, 120],
        "0":    [1.00, 1.50, 2.00],  # Standard
        "1":    [NaN, NaN, NaN],     # Zero memory allocation -> drop
        "2":    [NaN, 10.0, 10.0],   # At least 1 memory allocation
        "3":    [1.0, 2.0, 3.555],   # Test 3dp precision
    })

    expected_df = pd.DataFrame({
        "HashFunction":             [ "0",  "2",    "3"],
        "HashOwner":                [ "0",  "0",    "0"],
        "HashApp":                  [ "0",  "2",    "3"],
        "Average":                  [ 1.5, 10.0,  2.185],
        "Count":                    [   3,    2,      3],
        "Minimum":                  [ 1.0, 10.0,    1.0],
        "Maximum":                  [ 2.0, 10.0,  3.555],
        "percentile_Average_0":     [1.00, 10.0,    1.0],
        "percentile_Average_1":     [1.01, 10.0,   1.02],
        "percentile_Average_25":    [1.25, 10.0,    1.5],
        "percentile_Average_50":    [ 1.5, 10.0,      2],
        "percentile_Average_75":    [1.75, 10.0, 2.7775],
        "percentile_Average_99":    [1.99, 10.0, 3.5239],
        "percentile_Average_100":   [2.00, 10.0,  3.555],
    })

    dur_df = generate_dur_df(input_df)
    pd.testing.assert_frame_equal(dur_df, expected_df)
