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
import random
import pytest
from pandas.testing import assert_frame_equal
from glob import glob

import matplotlib.pyplot as plt
import numpy as np
import pandas as pd
from tqdm import tqdm
from typing import List, Dict

# from sampler.sample import random, sample_rollup, sample_single
from sampler.sample import Trace, plot_wasserstein_distances, get_rollup_samples, fold_samples, get_rolldown_samples

log.basicConfig(format='%(levelname)s:%(message)s', level=log.DEBUG)

random.seed(42)


def test_init_and_join():
    inv_df = pd.DataFrame(
        {
            "HashApp": ["ad", "ad"],
            "HashFunction": ["fe", "fd"],
            "HashOwner": ["ob", "ob"],
            "Trigger": ["tc", "tb"],
            "minute_118": [1, 2],
            "minute_119": [2, 1],
        }
    )
    mem_df = pd.DataFrame(
        {
            "HashFunction": ["fd", "fe"],
            "HashApp": ["ad", "ad"],
            "HashOwner": ["ob", "ob"],
            "SampleCount": [1, 1],
            "AverageAllocatedMb": [20.0, 10.0],
        }
    )
    run_df = pd.DataFrame(
        {
            "HashFunction": ["fd", "fe"],
            "HashOwner": ["ob", "ob"],
            "HashApp": ["ad", "ad"],
            "Average": [20, 10],
            "Count": [1, 1],
            "Minimum": [8, 6],
        }
    )

    joined_df_ref = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd"],
            "Average": [10, 20],
            "Minimum": [6, 8],
            "AverageAllocatedMb": [10.0, 20.0],
            "HashApp": ["ad", "ad"],
            "HashOwner": ["ob", "ob"],
            "Trigger": ["tc", "tb"],
            "minute_118": [1, 2],
            "minute_119": [2, 1],
            "Run_x_Mem": [100.0, 400.0],
        }
    )

    resources_df_ref = pd.DataFrame(
        {
            "HashApp": ["ad", "ad"],
            "HashFunction": ["fe", "fd"],
            "HashOwner": ["ob", "ob"],
            "Trigger": ["tc", "tb"],
            "minute_118": [0.0001, 0.0008],
            "minute_119": [0.0002, 0.0004],
        }
    )

    trace = Trace('origin', inv_df=inv_df, mem_df=mem_df, run_df=run_df, is_build_extra_dfs=True)

    log.debug(f"JoinedDF:\n{trace.joined_df}\n"
              f"ResourcesDF:\n{trace.resources_df}")

    assert_frame_equal(trace.joined_df.sort_values('HashFunction', ignore_index=True),
                       joined_df_ref.sort_values('HashFunction', ignore_index=True), check_like=True)
    assert_frame_equal(trace.resources_df.sort_values('HashFunction', ignore_index=True),
                       resources_df_ref.sort_values('HashFunction', ignore_index=True), check_like=True)


def test_get_sample():
    inv_df = pd.DataFrame(
        {
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "Trigger": ["tc", "tb", "tf", "tg"],
            "minute_118": [1, 2, 1, 2],
            "minute_119": [2, 1, 2, 1],
        }
    )
    mem_df = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "SampleCount": [1, 1, 1, 1],
            "AverageAllocatedMb": [10.0, 20.0, 10.0, 20.0],
        }
    )
    run_df = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "Average": [10, 20, 10, 20],
            "Count": [1, 1, 1, 1],
            "Minimum": [6, 8, 10, 12],
        }
    )

    trace = Trace('origin', inv_df=inv_df, mem_df=mem_df, run_df=run_df, is_build_extra_dfs=True)

    sample = trace.get_sample(2, 's2')

    for df in [sample.joined_df, sample.resources_df, sample.inv_df, sample.mem_df, sample.run_df]:
        assert len(df) == 2, "The sample's df-s should each have exactly 2 rows"


def test_fold_traces():
    inv_df_origin = pd.DataFrame(
        {
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "Trigger": ["tc", "tb", "tf", "tg"],
            "minute_118": [1, 2, 1, 2],
            "minute_119": [2, 1, 2, 1],
        }
    )
    mem_df_origin = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "SampleCount": [1, 1, 1, 1],
            "AverageAllocatedMb": [10.0, 20.0, 10.0, 20.0],
        }
    )
    run_df_origin = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "Average": [10, 20, 10, 20],
            "Count": [1, 1, 1, 1],
            "Minimum": [6, 8, 10, 12],
        }
    )
    ###################################
    inv_df1 = pd.DataFrame(
        {
            "HashApp": ["ad"],
            "HashFunction": ["fe"],
            "HashOwner": ["ob"],
            "Trigger": ["tc"],
            "minute_118": [1],
            "minute_119": [2],
        }
    )
    mem_df1 = pd.DataFrame(
        {
            "HashFunction": ["fe"],
            "HashApp": ["ad"],
            "HashOwner": ["ob"],
            "SampleCount": [1],
            "AverageAllocatedMb": [10.0],
        }
    )
    run_df1 = pd.DataFrame(
        {
            "HashFunction": ["fe"],
            "HashOwner": ["ob"],
            "HashApp": ["ad"],
            "Average": [10],
            "Count": [1],
            "Minimum": [6],
        }
    )
    ###################################
    inv_df2 = pd.DataFrame(
        {
            "HashApp": ["ad"],
            "HashFunction": ["fd"],
            "HashOwner": ["ob"],
            "Trigger": ["tb"],
            "minute_118": [2],
            "minute_119": [1],
        }
    )
    mem_df2 = pd.DataFrame(
        {
            "HashFunction": ["fd"],
            "HashApp": ["ad"],
            "HashOwner": ["ob"],
            "SampleCount": [1],
            "AverageAllocatedMb": [20.0],
        }
    )
    run_df2 = pd.DataFrame(
        {
            "HashFunction": ["fd"],
            "HashOwner": ["ob"],
            "HashApp": ["ad"],
            "Average": [20],
            "Count": [1],
            "Minimum": [8],
        }
    )
    ###################################
    inv_df_ref = pd.DataFrame(
        {
            "HashApp": ["ad", "ad"],
            "HashFunction": ["fe", "fd"],
            "HashOwner": ["ob", "ob"],
            "Trigger": ["tc", "tb"],
            "minute_118": [1, 2],
            "minute_119": [2, 1],
        }
    )
    mem_df_ref = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd"],
            "HashApp": ["ad", "ad"],
            "HashOwner": ["ob", "ob"],
            "SampleCount": [1, 1],
            "AverageAllocatedMb": [10.0, 20.0],
        }
    )
    run_df_ref = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd"],
            "HashOwner": ["ob", "ob"],
            "HashApp": ["ad", "ad"],
            "Average": [10, 20],
            "Count": [1, 1],
            "Minimum": [6, 8],
        }
    )
    ###################################
    ###################################
    trace = Trace('origin', inv_df=inv_df_origin, mem_df=mem_df_origin, run_df=run_df_origin, is_build_extra_dfs=True)
    trace1: Trace = Trace(name="tr1", inv_df=inv_df1, mem_df=mem_df1, run_df=run_df1, is_build_extra_dfs=True)
    trace2: Trace = Trace(name="tr2", inv_df=inv_df2, mem_df=mem_df2, run_df=run_df2, is_build_extra_dfs=True)

    resulting_trace: Trace = fold_samples(target_sample=trace1, subsample=trace2, original_trace=trace)

    assert_frame_equal(resulting_trace.inv_df.sort_values('HashFunction', ignore_index=True),
                       inv_df_ref.sort_values('HashFunction', ignore_index=True), check_like=True)
    assert_frame_equal(resulting_trace.mem_df.sort_values('HashFunction', ignore_index=True),
                       mem_df_ref.sort_values('HashFunction', ignore_index=True), check_like=True)
    assert_frame_equal(resulting_trace.run_df.sort_values('HashFunction', ignore_index=True),
                       run_df_ref.sort_values('HashFunction', ignore_index=True), check_like=True)


def test_get_inclusive_samples():
    inv_df = pd.DataFrame(
        {
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "Trigger": ["tc", "tb", "tf", "tg"],
            "minute_118": [1, 2, 1, 2],
            "minute_119": [2, 1, 2, 1],
        }
    )
    mem_df = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "SampleCount": [1, 1, 1, 1],
            "AverageAllocatedMb": [10.0, 20.0, 10.0, 20.0],
        }
    )
    run_df = pd.DataFrame(
        {
            "HashFunction": ["fe", "fd", "ff", "fg"],
            "HashOwner": ["ob", "ob", "of", "og"],
            "HashApp": ["ad", "ad", "af", "ag"],
            "Average": [10, 20, 10, 20],
            "Count": [1, 1, 1, 1],
            "Minimum": [6, 8, 10, 12],
        }
    )

    trace = Trace('origin', inv_df=inv_df, mem_df=mem_df, run_df=run_df, is_build_extra_dfs=True)

    min_size = 1
    max_size = 4
    step = 2

    funcs_to_test = {'roll-up': get_rollup_samples,
                     'roll-down': get_rolldown_samples,
                     }

    for foo in funcs_to_test.keys():
        candidates = funcs_to_test[foo](trace=trace, original_trace=trace, min_size=min_size, max_size=max_size, step=step, trial_num=2)

        i = 0
        assert len(candidates) == 2, f"Wrong number of samples obtained, expected 2 but got {len(candidates)}"
        for size in range(min_size, max_size, step):
            candidate = candidates[size]['best']
            log.debug(f"Sample-{i}'s inv df:\n{candidate.inv_df}")
            for df in [candidate.joined_df, candidate.resources_df, candidate.inv_df, candidate.mem_df,
                       candidate.run_df]:
                assert df['HashFunction'].is_unique, f"Inclusive samples should not have duplicates, df:\n{df}"
                assert len(df) == candidate.size, "The sample's df-s should have the same size as the sample itself"

            i += 1


###########################################################################################
################# The tests below are used for sensitivity studies only ###################
###########################################################################################

@pytest.mark.skip(reason="To be run manually")
def test_sampler_full_trace():
    inv_f, mem_f, run_f = sorted(glob('../data-preproc/*'))
    inv_df = pd.read_csv(inv_f)
    mem_df = pd.read_csv(mem_f)
    run_df = pd.read_csv(run_f)

    original_trace = Trace(name="origin", inv_df=inv_df, mem_df=mem_df, run_df=run_df, is_build_extra_dfs=True)

    log.info(f"The original trace has {len(original_trace.joined_df)} functions")

    sample_sizes = [10, 100, 500, 1000, 2000, 4000, 5000, 10000, 20000, 40000]
    # trials = [2, 8, 32, 128, 512, 2048, 4096]
    trials = [4]

    fig, ax = plt.subplots()
    plt.ylim(0, 12)
    plt.xscale('log')
    ax.set_ylabel("Wasserstein distance")
    ax.set_xlabel("Sample size")
    plt.title("Wasserstein distance for various sample size and trial number")

    min_size = 1000
    max_size = 44000
    step = 1000

    for trial_num in trials:
        log.info(f"Computing distances for {trial_num} trials")
        candidates: Dict = {}

        candidates = get_rollup_samples(trace=original_trace,
                                        min_size=min_size,
                                        max_size=max_size,
                                        step=step,
                                        trial_num=trial_num)

        for size in range(min_size, max_size, step):
            candidates[size]['best'].save(path='../samples')

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

        plot_wasserstein_distances(ax=ax, candidates=candidates, line_title=f"{trial_num} trials")
        plt.legend(loc='upper right')
        fig.savefig(f"../wd.png")
