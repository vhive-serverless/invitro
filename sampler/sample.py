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

from __future__ import annotations

import concurrent.futures
import logging as log
import multiprocessing
from typing import Dict, Any

import numpy as np
import pandas as pd
from scipy import stats
from pathlib import Path
import matplotlib.pyplot as plt

# Specific to the Azure trace
FIRST_MINUTES_COLUMN_POSITION = 4

# Empirically selected so that both the invocation and resources Wass. distances
# converge similarly.
#
# Based on the following estimation:
# Resources distribution is multiplicative of duration (in ms, avg 1000ms),
# invocation count per minute (avg <10/min), and memory (in MB, avg 200MB)
RES_NORM_FACTOR = 1000 * 10 * 100


class Trace:
    name: str
    size: int
    # Data frames from the trace
    inv_df: pd.DataFrame
    run_df: pd.DataFrame
    mem_df: pd.DataFrame

    # Derived data frames
    # All data frames joined into one
    joined_df: pd.DataFrame
    # Same as inv_df but with invocation count x avg_runtime x avg_memory
    resources_df: pd.DataFrame

    # Wasserstein distance DF-s
    wd_df: pd.DataFrame

    # Assumes that only one trace is sampled at a time, however, repeatedly
    # to find the best sample
    executor: concurrent.futures.ThreadPoolExecutor

    def __init__(self,
                 name: str, inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame,
                 # optional args below (used only for derived samples)
                 joined_df=pd.DataFrame(),
                 resources_df=pd.DataFrame(),
                 is_build_extra_dfs=False,
                 ) -> None:
        log.debug(f"Initializing {name} trace...")
        self.name = name
        self.inv_df: pd.DataFrame = inv_df
        self.mem_df: pd.DataFrame = mem_df
        self.run_df: pd.DataFrame = run_df

        self.size = len(self.inv_df)  # a preprocessed trace has the same number of functions (rows) in all specs

        self.joined_df = joined_df
        self.resources_df = resources_df

        self.executor = concurrent.futures.ThreadPoolExecutor(max_workers=multiprocessing.cpu_count())

        if is_build_extra_dfs:
            # Only for the original trace: build the joined data frame with the resource usage specs
            # Derived (sampled) traces inherit populated DF-s
            self.__build_extra_dfs()

    def __build_extra_dfs(self):
        log.debug(f"Joining specs for {self.name}")
        log.debug(f"Original dfs:\nRun df:\n{self.run_df.head()}\n"
                  f"Mem df:\n{self.mem_df.head()}\n"
                  f"Inv df:\n{self.inv_df.head()}")
        self.joined_df = self.inv_df \
            .merge(self.mem_df, how='inner', on=['HashFunction']) \
            .merge(self.run_df, how='inner', on=['HashFunction'])

        # Drop HashApp, HashOwner and other useless columns, which repeat in all df-s
        for regexp in ['_x$', '_y$', 'Count', 'SampleCount']:
            self.joined_df.drop(self.joined_df.filter(regex=regexp).columns, axis=1, inplace=True)

        self.joined_df['Run_x_Mem'] = self.joined_df.Average * self.joined_df.AverageAllocatedMb
        log.debug(f"Joined df:\n{self.joined_df}")

        self.resources_df = self.inv_df.copy()
        self.resources_df.iloc[:, FIRST_MINUTES_COLUMN_POSITION:] = \
            self.resources_df.iloc[:, FIRST_MINUTES_COLUMN_POSITION:] \
                .multiply(self.joined_df['Run_x_Mem'], axis="index")

        # Divide by a normalization factor (to bring the resources series close to the invocation series)
        self.resources_df.iloc[:, FIRST_MINUTES_COLUMN_POSITION:] /= RES_NORM_FACTOR

        log.debug(f"Resources df:\n{self.resources_df}")

    # Get a random sample from self
    # Note: recomputes distances from original_trace (if not None) or from self (otherwise)
    def get_sample(self, size: int, name: str, original_trace=None, exclude_funcs: list = []) -> Trace:
        log.debug("Getting sample {name}")

        # exclude functions (e.g., for inclusive sampling)
        inv_df = self.inv_df
        if len(exclude_funcs) > 0:
            inv_df = self.inv_df[~inv_df.HashFunction.isin(exclude_funcs)]

        # Sample
        inv_df = inv_df.sample(n=size)

        func_hashes = inv_df.HashFunction.to_list()

        # Sample other df-s according to the invoc sample
        mem_df = self.mem_df[self.mem_df.HashFunction.isin(func_hashes)]
        run_df = self.run_df[self.run_df.HashFunction.isin(func_hashes)]
        joined_df = self.joined_df[self.joined_df.HashFunction.isin(func_hashes)]
        resources_df = self.resources_df[self.resources_df.HashFunction.isin(func_hashes)]

        log.debug(f"Inv:\n{inv_df.head()}\n"
                  f"Mem:\n{mem_df.head()}\n"
                  f"Run:\n{run_df.head()}\n"
                  f"Joined:\n{joined_df.head()}\n"
                  f"Res:\n{resources_df.head()}"
                  )

        sample = Trace(name=name, inv_df=inv_df, mem_df=mem_df, run_df=run_df,
                       joined_df=joined_df,
                       resources_df=resources_df,
                       )

        if original_trace is None:
            sample.wd_df = compute_distances(original_trace=self, sample_trace=sample)
        else:
            sample.wd_df = compute_distances(original_trace=original_trace, sample_trace=sample)

        return sample

    def get_best_sample(self, trials: int, size: int, original_trace=None, exclude_funcs: list = []) -> Dict:
        avg_inv_wd_min = 0.0
        avg_res_wd_min = 0.0
        avg_avg_wd_min = 0.0
        candidates = {'stats': {'inv': [], 'res': [], 'avg': []}}

        futures = {}
        for i in range(trials):
            futures[i] = self.executor.submit(self.get_sample, size, f"s{size}-{i}", original_trace, exclude_funcs)

        for i in range(trials):
            candidate = futures[i].result()

            cur_inv_mean = np.mean(candidate.wd_df['Inv_wd'])
            cur_res_mean = np.mean(candidate.wd_df['Res_wd'])
            cur_avg_mean = np.mean(candidate.wd_df['Avg_wd'])

            candidates['stats']['inv'].append(np.mean(candidate.wd_df['Inv_wd']))
            candidates['stats']['res'].append(np.mean(candidate.wd_df['Res_wd']))
            candidates['stats']['avg'].append(np.mean(candidate.wd_df['Avg_wd']))

            log.debug(f"Cur inv/res/avg={cur_inv_mean:.2f}/{cur_res_mean:.2f}/{cur_avg_mean:.2f} VS "
                      f"Min {avg_inv_wd_min:.2f}/{avg_res_wd_min:.2f}/{avg_avg_wd_min:.2f}")

            if avg_inv_wd_min == 0.0 or \
                    (cur_avg_mean < avg_avg_wd_min):
                avg_inv_wd_min = cur_inv_mean
                avg_res_wd_min = cur_res_mean
                avg_avg_wd_min = cur_avg_mean
                candidates['best'] = candidate

        log.debug(f"Size-{size}, "
                  f"best_inv_wd={avg_inv_wd_min:.2f}, "
                  f"best_res_wd={avg_res_wd_min:.2f}, "
                  f"best_avg_wd={avg_avg_wd_min:.2f}, "
                  f"avg_inv_wd={np.mean(candidates['stats']['inv']):.2f}, "
                  f"avg_res_wd={np.mean(candidates['stats']['res']):.2f}, "
                  f"std_inv_wd={np.std(candidates['stats']['inv']):.2f}, "
                  f"std_res_wd={np.std(candidates['stats']['res']):.2f}, "
                  )

        return candidates

    def save(self, path: str) -> None:
        log.debug("Save trace files")
        full_path: str = f"{path}/{self.size}"

        Path(f"{full_path}/").mkdir(parents=True, exist_ok=True)

        self.inv_df.to_csv(f"{full_path}/invocations.csv", index=False)
        self.mem_df.to_csv(f"{full_path}/memory.csv", index=False)
        self.run_df.to_csv(f"{full_path}/durations.csv", index=False)


def compute_distances(original_trace: Trace, sample_trace: Trace) -> pd.DataFrame:
    log.debug(f"Computing distances for sample {sample_trace.name}")
    log.debug(f"Compute WS for a sample:\n{sample_trace.inv_df.head()}")

    wd_list = []

    for col in sample_trace.inv_df.columns[FIRST_MINUTES_COLUMN_POSITION:]:
        inv = stats.wasserstein_distance(sample_trace.inv_df[col], original_trace.inv_df[col])
        res = stats.wasserstein_distance(sample_trace.resources_df[col], original_trace.resources_df[col])

        wd_list.append([col, inv, res, (inv + res) / 2])
        log.debug(f"Computed W distances for min-{col}: inv:{inv:.2f}, res:{res:.2f}, avg:{(inv + res) / 2}")

    return pd.DataFrame(wd_list, columns=['Minute', 'Inv_wd', 'Res_wd', 'Avg_wd'])


def plot_wasserstein_distances(ax, candidates: Dict, line_title: str):
    wd_mean = {}
    wd_std = {}
    for size in sorted(candidates.keys()):
        wd_mean[size] = candidates[size]['best'].wd_df['Avg_wd'].mean()
        wd_std[size] = np.std(candidates[size]['best'].wd_df['Avg_wd'])

    x, y = zip(*(sorted(wd_mean.items())))
    x_std, std = zip(*(sorted(wd_std.items())))

    ax.errorbar(x, y, std, label=f"{line_title}")


# Folds the subsample into the target sample, by appending their DF-s, and returns the folded sample.
def fold_samples(target_sample: Trace, subsample: Trace, original_trace: Trace) -> Trace:
    log.info(f"Include subsample w/ {subsample.size} into sample w/ {target_sample.size}")
    log.debug(f"The sample before inclusion\n"
              f"Inv:\n{target_sample.inv_df.head()}\n"
              f"The subsample to be included\n"
              f"Inv:\n{subsample.inv_df.head()}\n"
              )

    inv_df = target_sample.inv_df.append(subsample.inv_df, ignore_index=True)
    mem_df = target_sample.mem_df.append(subsample.mem_df, ignore_index=True)
    run_df = target_sample.run_df.append(subsample.run_df, ignore_index=True)

    joined_df = target_sample.joined_df.append(subsample.joined_df, ignore_index=True)
    resources_df = target_sample.resources_df.append(subsample.resources_df, ignore_index=True)

    resulting_sample: Trace = Trace(name=f"s{target_sample.size + subsample.size}",
                                    inv_df=inv_df,
                                    mem_df=mem_df,
                                    run_df=run_df,
                                    joined_df=joined_df,
                                    resources_df=resources_df)

    # after changing the DF-s, need to re-compute the distances
    resulting_sample.wd_df = compute_distances(original_trace=original_trace, sample_trace=target_sample)

    assert resulting_sample.size == len(resulting_sample.inv_df), \
        "The effective sample size must be equal to the sum of  df lengths"

    log.debug(f"After folding\n"
              f"Inv:\n{resulting_sample.inv_df.head()}\n"
              f"Mem:\n{resulting_sample.mem_df.head()}\n"
              f"Run:\n{resulting_sample.run_df.head()}\n"
              f"Joined:\n{resulting_sample.joined_df.head()}\n"
              f"Resources:\n{resulting_sample.resources_df.head()}\n"
              )

    return resulting_sample


##########################################################################
# The roll-up and roll-down functions both generate samples in the way
# that samples recursively include all the samples smaller than them.
##########################################################################

# Rolls up biggest samples out of iteratively chosen smallest ones
def get_rollup_samples(trace: Trace, original_trace: Trace, min_size: int, max_size: int, step: int, trial_num: int) -> dict[
    str | int, dict[str, list] | dict | dict[str, Trace | dict[str, Any]]]:
    candidates = {}

    cur_size = min_size

    log.info(f"Performing roll-up sampling\n"
             f"Obtaining sample w/ {cur_size}")
    cur_candidate = trace.get_best_sample(trials=trial_num, size=cur_size, original_trace=original_trace)

    for size in range(min_size, max_size, step):

        if size == min_size:
            candidates[size] = cur_candidate
            continue

        delta_size = size - cur_size
        log.info(f"Obtaining sample w/ {size}, by taking a delta sample w/ {delta_size}")

        # avoid sampling the same functions
        exclude_funcs = cur_candidate['best'].inv_df.HashFunction.to_list()

        delta_sample_candidate = trace.get_best_sample(
            trials=trial_num,
            size=delta_size,
            exclude_funcs=exclude_funcs,
            original_trace=original_trace)

        folded_trace = fold_samples(
            target_sample=cur_candidate['best'],
            subsample=delta_sample_candidate['best'],
            original_trace=original_trace)

        cur_candidate = {
            'best': folded_trace,
            'stats': {
                'inv': np.mean(
                    [np.mean(cur_candidate['stats']['inv']), np.mean(delta_sample_candidate['stats']['inv'])]),
                'res': np.mean(
                    [np.mean(cur_candidate['stats']['res']), np.mean(delta_sample_candidate['stats']['res'])]),
                'avg': np.mean(
                    [np.mean(cur_candidate['stats']['avg']), np.mean(delta_sample_candidate['stats']['avg'])]),
            },
        }

        candidates[size] = cur_candidate.copy()

        cur_size = size

    return candidates


# Takes samples recursively starting from the largest sample,
# i.e., at each step a smaller sample is taken from the previous sample
def get_rolldown_samples(trace: Trace, original_trace: Trace, min_size: int, max_size: int, step: int, trial_num: int
                         ) -> dict[str | int, dict[str, list] | dict | dict[str, Trace | dict[str, Any]]]:
    candidates = {}

    cur_size = max_size

    log.info(f"Roll-down sampling mode: Obtaining sample w/ {cur_size}")
    cur_candidate = trace.get_best_sample(trials=trial_num, size=cur_size, original_trace=original_trace)

    for size in reversed(range(min_size, max_size, step)):

        if size == max_size:
            candidates[size] = cur_candidate
            continue

        log.info(f"Obtaining sample w/ {size}")

        next_candidate = cur_candidate['best'].get_best_sample(
            trials=trial_num,
            size=size,
            original_trace=original_trace,
        )

        candidates[size] = next_candidate.copy()

        cur_candidate = next_candidate

    return candidates


def generate_samples(inv_df: pd.DataFrame, mem_df: pd.DataFrame, run_df: pd.DataFrame,
                     inv_df_orig: pd.DataFrame, mem_df_orig: pd.DataFrame, run_df_orig: pd.DataFrame,
                     min_size: int, step: int, max_size: int, trial_num: int,
                     out_path: str
                     ):
    original_trace = Trace(name="origin", inv_df=inv_df_orig, mem_df=mem_df_orig, run_df=run_df_orig, is_build_extra_dfs=True)
    source_trace = Trace(name="source", inv_df=inv_df, mem_df=mem_df, run_df=run_df, is_build_extra_dfs=True)

    log.info(f"The original trace has {source_trace.size} functions, "
             f"the best sample is selected based on {trial_num} attempts for each sample size")

    candidates = get_rolldown_samples(trace=source_trace, original_trace=original_trace, min_size=min_size, max_size=max_size, step=step, trial_num=trial_num)

    for size in range(min_size, max_size, step):
        candidates[size]['best'].save(path=f"{out_path}/samples")

    log.info(f"Stats per sample:")
    for s in range(min_size, max_size, step):
        best_inv_wd = np.mean(candidates[s]['best'].wd_df['Inv_wd'])
        best_res_wd = np.mean(candidates[s]['best'].wd_df['Res_wd'])
        best_avg_wd = np.mean(candidates[s]['best'].wd_df['Avg_wd'])
        improvement_ratio = np.mean(candidates[s]['stats']['avg']) / best_avg_wd
        log.info(f"{candidates[s]['best'].name},"
                  f"inv={best_inv_wd:.2f},"
                  f"res={best_res_wd:.2f},"
                  f"avg={best_avg_wd:.2f},"
                  f"impr={improvement_ratio:.2f}")

    # Plotting Wasserstein distance for a sweep of samples
    fig, ax = plt.subplots()
    plt.ylim(0, 12)
    plt.xscale('log')
    ax.set_ylabel("Wasserstein distance")
    ax.set_xlabel("Sample size")
    plt.title("Wasserstein distance for various sample size and trial number")

    plot_wasserstein_distances(ax=ax, candidates=candidates, line_title=f"{trial_num} trials")

    plt.legend(loc='upper right')
    fig.savefig(f"{out_path}/wd.png")
