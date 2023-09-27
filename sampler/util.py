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

import logging as log
import glob

import pandas as pd
from typing import Tuple

us_to_s = 10 ** -6

log.basicConfig(
    level=log.INFO,
    format='(%(asctime)s) Trace sampler -- [%(levelname)s] %(message)s'
)


# Reads
def read_trace_dataframes(path: str) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    inv_file = glob(f"{path}/*invocations*.csv")
    assert len(inv_file) >= 1, "Invocations file does not exist"
    assert len(inv_file) == 1, "There are too many possible invocations files"
    inv_df = pd.read_csv(inv_file[0])

    mem_file = glob(f"{path}/*memory*.csv")
    assert len(mem_file) >= 1, "Memory file does not exist"
    assert len(mem_file) == 1, "There are too many possible memory files"
    mem_df = pd.read_csv(mem_file[0])

    run_file = glob(f"{path}/*durations*.csv")
    assert len(run_file) >= 1, "Runtime file does not exist"
    assert len(run_file) == 1, "There are too many possible runtime files"
    run_df = pd.read_csv(run_file[0])

    return inv_df, mem_df, run_df
