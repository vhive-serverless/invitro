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

import pandas as pd
import logging as log

from sampler.filter import filter_azure2021
import os

def create_original_df_file(tmp_path):

    og_df = pd.DataFrame(
        {
            "app":           ["aa", "ab", "ac", "ac", "ac"],
            "func":          ["fa", "fb", "fc", "fd", "fd"],
            "end_timestamp": [1.00, 10.0, 80.0, 100.0, 300.0], # 5 Minutes, 0-300 seconds
            "duration":      [0.50,  5.0, 15.5,  40.0, 100.0],
        # "start_timestamp": [0.50,  5.0, 64.5,  60.0, 200.0]
        }
    )

    dir_path = tmp_path / "og_df"
    dir_path.mkdir()

    og_df.to_csv(dir_path / "AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt", index=False)

    return str(dir_path)

def create_sampled_df_file(tmp_path):

    inv_df = pd.DataFrame(
        {
            "HashApp":      ["aa", "ab", "ac"],
            "HashFunction": ["fa", "fb", "fd"],
            "HashOwner":    ["oa", "ob", "oc"],
            "Trigger":      ["tr", "tr", "tr"],
            "1":            [1, 1, 0],
            "2":            [0, 0, 1],
            "3":            [0, 0, 0],
            "4":            [0, 0, 1],
            "5":            [0, 0, 0],
        }
    )

    dir_path = tmp_path / "sampled_df"
    dir_path.mkdir()

    inv_df.to_csv(dir_path / "invocations.csv", index=False)

    return str(dir_path)

# Unit happy path test.
def test_azure2021_filter(tmp_path):

    orig_trace_dir = create_original_df_file(tmp_path)
    sampled_trace_dir = create_sampled_df_file(tmp_path)
    out_dir = tmp_path / "output"
    start_time = "00:00:00"
    duration = 5
    
    filter_azure2021(orig_trace_dir, sampled_trace_dir, str(out_dir), start_time, duration)

    filtered_df_path = out_dir / "SampledAzure2021.csv"
    filtered_df = pd.read_csv(filtered_df_path)

    expected_filtered_df = pd.DataFrame(
        {
            "app":           ["aa", "ab", "ac", "ac"],
            "func":          ["fa", "fb", "fd", "fd"],
            "end_timestamp": [1.00, 10.0, 100.0, 300.0],
            "duration":      [0.50,  5.0,  40.0, 100.0],
        # "start_timestamp": [0.50,  5.0,  60.0, 200.0]
        }
    )

    pd.testing.assert_frame_equal(filtered_df, expected_filtered_df)

