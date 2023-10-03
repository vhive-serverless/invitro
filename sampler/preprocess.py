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

from glob import glob
from tqdm import tqdm
from typing import Tuple

FIRST_MEMORY_COLUMN_POSITION = 4


def parse_trace_files(trace: str, starting_day: int, hours: int, minutes: int, dur: int
                      ) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    if starting_day < 10:
        d = "0" + str(starting_day)
    else:
        d = str(starting_day)
    inv_file = glob(f"{trace}/*invocations*{d}*.csv")
    assert len(inv_file) >= 1, "Invocations file does not exist"
    assert len(inv_file) == 1, "There are too many possible invocations files"
    inv_df = pd.read_csv(inv_file[0])

    mem_file = glob(f"{trace}/*memory*{d}*.csv")
    assert len(mem_file) >= 1, "Memory file does not exist"
    assert len(mem_file) == 1, "There are too many possible memory files"
    mem_df = pd.read_csv(mem_file[0])

    run_file = glob(f"{trace}/*durations*{d}*.csv")
    assert len(run_file) >= 1, "Runtime file does not exist"
    assert len(run_file) == 1, "There are too many possible runtime files"
    run_df = pd.read_csv(run_file[0])

    # only invocations dataframe gets sliced, others have no time component
    inv_df = get_inv_time_slice(inv_df, hours, minutes, dur)

    inv_df, mem_df, run_df = transform_dfs(inv_df=inv_df, mem_df=mem_df, run_df=run_df)

    log.info(f"The cleaned slice files contain rows: "
             f"inv={len(inv_df)}, mem={len(mem_df)}, run={len(run_df)}")

    ok, msg = validate_output_dfs(inv_df=inv_df, mem_df=mem_df, run_df=run_df)

    if not ok:
        log.fatal(f"Output dataframes are inconsistent: {msg}")

    return inv_df, run_df, mem_df


# Cleans the data frames and expands the memory dataframe in a per-func fashion
def transform_dfs(
        inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    log.info(f"The slice files contain rows: inv={len(inv_df)}, mem={len(mem_df)}, run={len(run_df)}")

    inv_df, mem_df, run_df = remove_duplicates(inv_df=inv_df, mem_df=mem_df, run_df=run_df)

    inv_df = remove_uninvoked(inv_df=inv_df)

    run_df = remove_zero_duration(run_df=run_df)

    inv_df, mem_df, run_df = get_intersections(inv_df=inv_df, mem_df=mem_df, run_df=run_df)

    mem_df = build_mem_func_df(mem_df=mem_df, run_df=run_df)

    return inv_df, mem_df, run_df


# Removes rows with duplicate HashApp and HashFunction fields for mem df and inv/run df-s, respectively
def remove_duplicates(
        inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    dup_list = inv_df.duplicated(subset=['HashFunction'], keep='first')
    inv_df_cleaned = inv_df[~dup_list]
    log.info(f"Inv df deduplicated (before -> after): {len(inv_df)} -> {len(inv_df_cleaned)}")
    log.debug(f"Deduplicated inv df:\n{inv_df_cleaned}")

    dup_list = mem_df.duplicated(subset=['HashApp'], keep='first')
    mem_df_cleaned = mem_df[~dup_list]
    log.info(f"Mem df deduplicated (before -> after): {len(mem_df)} -> {len(mem_df_cleaned)}")
    log.debug(f"Deduplicated mem df:\n{mem_df_cleaned}")

    dup_list = run_df.duplicated(subset=['HashFunction'], keep='first')
    run_df_cleaned = run_df[~dup_list]
    log.info(f"Run df deduplicated (before -> after): {len(run_df)} -> {len(run_df_cleaned)}")
    log.debug(f"Deduplicated run df:\n{run_df_cleaned}")

    return inv_df_cleaned, mem_df_cleaned, run_df_cleaned

# Removes uninvoked functions from invocations dataframe
# Respective entries from memory and durations dataframes will be filtered in get_intersection()
def remove_uninvoked(inv_df: pd.DataFrame) -> pd.DataFrame:
    uninvoked = inv_df.drop(['Trigger', 'HashOwner', 'HashFunction', 'HashApp'], axis='columns').sum(axis='columns') == 0
    inv_df_cleaned = inv_df[~uninvoked]

    return inv_df_cleaned

# Removes functions with an average execution time of 0 ms
# Respective entries from memory and invocations dataframes will be filtered in get_instersection()
def remove_zero_duration(run_df: pd.DataFrame) -> pd.DataFrame:
    zero_duration = run_df.Average == 0
    run_df_cleaned = run_df[~zero_duration]

    return run_df_cleaned

# Expands memory file with per-function memory usage (instead of the per-app as in the original trace)
# Each function of an app uses proportional fraction of memory
def build_mem_func_df(mem_df: pd.DataFrame, run_df: pd.DataFrame) -> pd.DataFrame:
    log.info(f"Building per-func memory specs")
    mem_df['HashFunction'] = mem_df['HashApp']  # add one more column (populated later)
    # move the added column to the left-most position
    columns = mem_df.columns.to_list()
    columns = columns[-1:] + columns[:-1]
    mem_df = mem_df[columns]

    mem_row_list = []

    for i, mem_row in tqdm(mem_df.iterrows(), total=len(mem_df)):
        log.debug(f"Processing mem row:\n{mem_row}")
        app_hash = mem_row.HashApp
        func_rows_run = run_df.loc[run_df['HashApp'] == app_hash]

        # Note: Hashes are guaranteed to match in run and inv dfs by the get_intersections() func
        func_hashes = func_rows_run.HashFunction.to_list()

        if len(func_hashes) == 0:
            log.debug(f"App hash {app_hash} not found in run & inv dfs")
            continue

        func_count = len(func_hashes)
        log.debug(f"Found {func_count} hashes:\n{func_hashes}")

        # Every function in an app gets proportional memory footprint
        mem_row[FIRST_MEMORY_COLUMN_POSITION:] = mem_row[FIRST_MEMORY_COLUMN_POSITION:] / func_count

        for func_hash in func_hashes:
            mem_row_tmp = mem_row.copy()
            mem_row_tmp.HashFunction = func_hash
            mem_row_list.append(mem_row_tmp)

    mem_func_df = pd.DataFrame(mem_row_list)

    mem_func_df = mem_func_df.reset_index(drop=True)
    log.debug(f"Built per-func mem df:\n{mem_func_df}")

    return mem_func_df


# Leaves only rows with the common HashApp and HashFunction values in all 3 dfs
def get_intersections(
        inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame
) -> Tuple[pd.DataFrame, pd.DataFrame, pd.DataFrame]:
    # Intersection by HashApp among inv, mem, run dfs
    inv_app_hashes = inv_df.HashApp.to_list()
    mem_app_hashes = mem_df.HashApp.to_list()
    run_app_hashes = run_df.HashApp.to_list()
    common_app_hashes = [v for v in inv_app_hashes if v in mem_app_hashes]
    common_app_hashes = [v for v in common_app_hashes if v in run_app_hashes]

    inv_df = inv_df[inv_df.HashApp.isin(common_app_hashes)].reset_index(drop=True)
    log.debug(f"Inv df after app hashes intersection:\n{inv_df}")
    mem_df = mem_df[mem_df.HashApp.isin(common_app_hashes)].reset_index(drop=True)
    log.debug(f"Mem df after app hashes intersection:\n{mem_df}")
    run_df = run_df[run_df.HashApp.isin(common_app_hashes)].reset_index(drop=True)
    log.debug(f"Run df after app hashes intersection:\n{run_df}")

    # Intersection by HashFunction among inv and run (note, mem df does not have HashFunction column)
    inv_func_hashes = inv_df.HashFunction.to_list()
    run_func_hashes = run_df.HashFunction.to_list()
    common_func_hashes = [v for v in inv_func_hashes if v in run_func_hashes]

    inv_df = inv_df[inv_df.HashFunction.isin(common_func_hashes)].reset_index(drop=True)
    log.debug(f"Inv df after func hashes intersection:\n{inv_df}")
    run_df = run_df[run_df.HashFunction.isin(common_func_hashes)].reset_index(drop=True)
    log.debug(f"Run df after func hashes intersection:\n{run_df}")

    return inv_df, mem_df, run_df


def validate_output_dfs(inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame) -> [bool, str]:
    msg: str
    ok = True

    if len(mem_df) != len(run_df):
        ok = False
        msg = "The number of rows in run and mem files must match"
        return ok, msg
    elif len(mem_df) != len(inv_df):
        ok = False
        msg = "The number of rows in inv and mem files must match"
        return ok, msg

    if inv_df.isna().sum().sum() != 0:
        ok = False
        msg = "There should be no NaN values in the inv df"
        return ok, msg
    elif run_df.isna().sum().sum() != 0:
        ok = False
        msg = "There should be no NaN values in the run mem df"
        return ok, msg
    elif mem_df.isna().sum().sum() != 0:
        ok = False
        msg = "There should be no NaN values in the mem df"
        return ok, msg

    return ok, ''


def get_inv_time_slice(inv_df: pd.DataFrame, h: int, m: int, dur: int) -> pd.DataFrame:
    start_min = 60 * h + m
    if start_min < 0:
        raise Exception("Starting hour and starting minute should not be negative")
    idx = 0  # index of first column containing invocations
    for i, col in enumerate(inv_df.columns):
        if str(col) == '1':
            idx = i
            break
    inv_df = inv_df.drop(inv_df.columns[idx:start_min + idx - 1],
                         axis=1)  # drop all invocation columns before starting min
    inv_df = inv_df.drop(inv_df.columns[idx + int(dur):], axis=1)

    inv_df = inv_df.dropna()

    return inv_df
