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


from sampler.preprocess import get_inv_time_slice, transform_dfs, validate_output_dfs


def create_df():
    data = ['a', 'b']
    data2 = ['d', 'e']
    data3 = ['g', 'h']
    d = {'HashOwner': data, 'HashApp': data2, 'HashFunction': data3}
    df = pd.DataFrame(d)
    for i in range(1, 16):
        df[str(i)] = [i, i * 2]
    return df


def test_time_slicing():
    # Create DataFrames
    inv_df = create_df()
    dur_min = 5
    inv_processed = get_inv_time_slice(inv_df=inv_df, h=0, m=10, dur=dur_min)

    assert len(inv_processed.columns) == 3 + dur_min
    # 3 for the first 3 columns which are kept, no warmup and no profiling
    assert len(inv_processed.index) == len(inv_df.index)
    # number of rows has to stay the same as in the original dataframe
    sum_invocations = 0
    for i in range(3, dur_min + 3):
        sum_invocations += inv_processed.iloc[:, i].sum()
    expected_sum = 3 * sum(range(10, 15))  # start at minute 10, end at minute 14
    assert sum_invocations == expected_sum


def test_input_cleaning():
    # The order of columns in the test DFs as in the original trace

    # Corruptions:
    # - The function "fa" is not invoked
    # - The app "ab" is not in mem df
    # - The function "fc" (app "ac") is not in mem df
    # - "fd" are "fe" functions of the same app
    # - The function "fe" has duplicated contains a duplicate function
    # - The function "ff" (app "ad") is not in run df although "ad" app is in both dfs
    # - The function "fg" is not in run df
    # - The function "fh" has an average execution time of 0 ms
    inv_df = pd.DataFrame(
        {
            "HashApp": ["aa", "ab", "ac", "ad", "ad", "ad", "ad", "ae", "af"],
            "HashFunction": ["fa", "fb", "fc", "fd", "fe", "fe", "ff", "fg", "fh"],
            "HashOwner": ["oa", "oa", "ob", "ob", "ob", "oc", "oc", "od", "oe"],
            "Trigger": ["tr", "tr", "tr", "tr", "tr", "tr", "tr", "tr", "tr"],
            "118": [0, 1, 2, 3, 4, 5, 6, 7, 8],
            "119": [0, 1, 2, 3, 4, 5, 6, 7, 8],
        }
    )

    inv_df_ref = pd.DataFrame(
        {
            "HashApp": ["ad", "ad"],
            "HashFunction": ["fd", "fe"],
            "HashOwner": ["ob", "ob"],
            "Trigger": ["tr", "tr"],
            "118": [3, 4],
            "119": [3, 4],
        }
    )

    # Corruptions:
    # - The first and penultimate rows have duplicate HashApp
    mem_df = pd.DataFrame(
        {
            "HashApp": ["aa", "ad", "aa", "ae", "af"],
            "HashOwner": ["oa", "ob", "oc", "od", "oe"],
            "SampleCount": [1, 1, 1, 1, 1],
            "AverageAllocatedMb": [20, 60, 80, 100, 120],
            "AverageAllocatedMb_pct1": [2, 6, 8, 10, 12],
        }
    )

    # Transformations:
    # - HashApp replaced by HashFunction
    # - Memory specs divided equally among functions of the same app
    mem_df_ref = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd"],
            "HashApp": ["ad", "ad"],
            "HashOwner": ["ob", "ob"],
            "SampleCount": [1, 1],
            "AverageAllocatedMb": [30.0, 30.0],
            "AverageAllocatedMb_pct1": [3.0, 3.0],
        }
    )

    # Corruptions:
    # - The HashFunction "fd" is duplicated
    # - Function "fa" is not in run df
    # - The app "ab" is not in mem df
    # - The app "af" has an average execution time of 0 ms
    run_df = pd.DataFrame(
        {
            "HashFunction": ["fa", "fb", "fe", "fd", "fd", "fh"],
            "HashOwner": ["oa", "oa", "ob", "ob", "oc", "oe"],
            "HashApp": ["aa", "ab", "ad", "ad", "ad", "af"],
            "Average": [10, 20, 30, 40, 50, 0],
            "Count": [1, 1, 1, 1, 1, 1],
            "Minimum": [4, 6, 8, 10, 12, 0],
        }
    )

    run_df_ref = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd"],
            "HashOwner": ["ob", "ob"],
            "HashApp": ["ad", "ad"],
            "Average": [30, 40],
            "Count": [1, 1],
            "Minimum": [8, 10],
        }
    )

    ################################################################################################

    inv_df, mem_df, run_df = transform_dfs(inv_df=inv_df, mem_df=mem_df, run_df=run_df)
    assert mem_df.compare(mem_df_ref).empty, f"Transformed mem df does not match:\n{mem_df}"

    log.info(f"The cleaned dfs contain rows: "
             f"inv={len(inv_df)}, mem={len(mem_df)}, run={len(run_df)}")

    ok, msg = validate_output_dfs(inv_df=inv_df, mem_df=mem_df, run_df=run_df)
    assert ok, "Validation of the cleaned dfs failed"

    assert inv_df.compare(inv_df_ref).empty, f"Transformed inv df does not match:\n{inv_df}"
    assert mem_df.compare(mem_df_ref).empty, f"Transformed mem df does not match:\n{mem_df}"
    assert run_df.compare(run_df_ref).empty, f"Transformed run df does not match:\n{run_df}"
