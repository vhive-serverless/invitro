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

import logging as log
import pandas as pd

import os
from glob import glob

from sampler.preprocess2021 import filter_within_time_interval

def filter_azure2021(orig_trace_dir: str, sampled_trace_dir: str, out_dir: str, start_time: str, duration: int, orig_trace_filename: str = None):
    if orig_trace_filename is None:
        orig_trace_filename = "AzureFunctionsInvocationTraceForTwoWeeksJan2021.txt"

    # Read original trace
    trace_file = glob(f"{orig_trace_dir}/{orig_trace_filename}")
    assert len(trace_file) == 1, "Expected 1 Azure2021 trace file"
    trace_df = pd.read_csv(trace_file[0])

    # Read sampled trace
    invocations_file = glob(f"{sampled_trace_dir}/invocations.csv")
    assert len(invocations_file) == 1, "Expected 1 invocations file"
    inv_df = pd.read_csv(invocations_file[0])

    # Filter original trace to keep sampled functions
    trace_df, _, _ = filter_within_time_interval(trace_df, start_time, duration)

    inv_df = inv_df.rename(columns={'HashApp': 'app', 'HashFunction': 'func'})
    functions_to_keep_keys = inv_df[['app', 'func']].drop_duplicates()
    trace_df = trace_df.merge(functions_to_keep_keys, on=['app', 'func'], how='inner')

    # Cleanup
    trace_df = trace_df.drop(columns=['start_timestamp'])

    log.info(f"The final sampled trace contains: {len(trace_df)} rows and {len(trace_df.groupby(['app', 'func']))} functions")

    # Save to file at directory
    if not os.path.exists(out_dir):
        try:
            os.makedirs(out_dir)
        except OSError as e:
            raise RuntimeError(f"Failed to create the output folder: {e}")
        
    log.info(f"Saving sampled Azure2021 to {out_dir}/SampledAzure2021.csv")
    trace_df.to_csv(f"{out_dir}/SampledAzure2021.csv", index=False)

if __name__ == "__main__":
    orig_trace_dir = "data/azure2021/"
    sampled_trace_dir = "data/traces/reference/sampledAzure2021/" + "samples/40"
    out_dir = "data/traces/reference/filtered2021"
    start_time = "00:01:00"
    duration = 100

    log.basicConfig(format='%(levelname)s:%(message)s', level=log.INFO)

    filter_azure2021(orig_trace_dir, sampled_trace_dir, out_dir, start_time, duration)

