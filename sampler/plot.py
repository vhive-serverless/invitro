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

import os

import matplotlib.pyplot as plt
import seaborn as sns


def plot_cdf(kind, output, inv_df, run_df, mem_df, sample):
    if not os.path.exists(f"{output}"):
        os.makedirs(f"{output}")

    if kind == 'invocations':
        plot_invocations(output, inv_df, sample)
    elif kind == 'durations':
        plot_execution_time(output, run_df, sample)
    elif kind == 'memory':
        plot_memory(output, mem_df, sample)
    else:
        raise RuntimeError('Unsupported type: ', kind)
    return

def plot_invocations(output, inv_df, inv_sample):
    sample_data = inv_sample.iloc[:, 4:]
    sample_hours = sample_data.T.groupby(sample_data.T.reset_index(drop=True).index//60).sum().T
    sample_data = sample_hours
    invdf = inv_df.iloc[:, 4:]
    inv_hours = invdf.T.groupby(invdf.T.reset_index(drop=True).index//60).sum().T
    inv_data = inv_hours
    inv_mean = inv_data.T.mean()
    inv_mean = inv_mean.reset_index(drop=True)
    inv_sum = inv_data.T.sum()
    inv_sum = inv_sum.reset_index(drop=True)
    inv_max = inv_data.T.max()
    inv_max = inv_max.reset_index(drop=True)
    ax=sns.ecdfplot(data=inv_data.T.mean(), log_scale=True, c="r", label='Trace', alpha=0.8)
    sns.ecdfplot(ax=ax,data=sample_data.T.mean(), log_scale=True, c="b", label='Sample', alpha=0.8)
    ax.legend()
    plt.savefig(f"{output}/inv.png")
    return
    
def plot_memory(output, mem_df, mem_sample):
    log = True
    ax = sns.ecdfplot(  data=mem_sample, x="AverageAllocatedMb", label="sample-mean", color='b', ls='-', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=mem_sample, x="AverageAllocatedMb_pct1", label="sample-min", color='b', ls=':', alpha=0.9, log_scale=log)
    sns.ecdfplot(ax=ax, data=mem_sample, x="AverageAllocatedMb_pct100", label="sample-max", color='b', ls='--', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=mem_df, x="AverageAllocatedMb", color='r', label="trace-mean", ls='-', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=mem_df, x="AverageAllocatedMb_pct1", color='r', label="trace-min", ls=':', alpha=0.9, log_scale=log)
    sns.ecdfplot(ax=ax, data=mem_df, x="AverageAllocatedMb_pct100", color='r', label="trace-max", ls='--', alpha=0.7, log_scale=log)
    ax.legend()
    plt.savefig(f"{output}/mem.png")
    return

def plot_execution_time(output, run_df, run_sample):
    log = True
    ax= sns.ecdfplot(   data=run_df, x="percentile_Average_50", color='r', label="trace-median", ls='-', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=run_df, x="Minimum", color='r', label="trace-min", ls=':', alpha=0.9, log_scale=log)
    sns.ecdfplot(ax=ax, data=run_df, x="Maximum", color='r', label="trace-max", ls='--', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=run_sample, x="percentile_Average_50", label="sample-median", color='b', ls='-', alpha=0.7, log_scale=log)
    sns.ecdfplot(ax=ax, data=run_sample, x="Minimum", label="sample-min", color='b', ls=':', alpha=0.9, log_scale=log)
    sns.ecdfplot(ax=ax, data=run_sample, x="Maximum", label="sample-max", color='b', ls='--', alpha=0.7, log_scale=log)
    ax.legend()
    plt.savefig(f"{output}/run.png")
    return
